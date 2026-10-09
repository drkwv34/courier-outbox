package worker

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/drkwv34/courier-outbox/internal/domain"
	"github.com/drkwv34/courier-outbox/internal/sign"
)

// Store is the persistence port the worker loop needs.
type Store interface {
	ClaimDue(ctx context.Context, workerID string, lease time.Duration, limit int) ([]domain.DeliveryClaim, error)
	RecordAttempt(ctx context.Context, rec domain.AttemptRecord) error
}

// Pool claims due deliveries, POSTs signed payloads, and records outcomes.
type Pool struct {
	store       Store
	envelope    domain.Envelope
	client      *http.Client
	logger      *slog.Logger
	id          string
	concurrency int
	claimLimit  int
	lease       time.Duration
	poll        time.Duration
	backoff     []time.Duration
	now         func() time.Time
	requestID   func() string
}

// Config is the injected worker runtime configuration.
type Config struct {
	Store       Store
	Envelope    domain.Envelope
	Logger      *slog.Logger
	Client      *http.Client
	WorkerID    string
	Concurrency int
	ClaimLimit  int
	Lease       time.Duration
	Poll        time.Duration
	Backoff     []time.Duration
	Now         func() time.Time
}

// NewPool validates deps and returns a ready pool.
func NewPool(cfg Config) (*Pool, error) {
	if cfg.Store == nil {
		return nil, fmt.Errorf("worker: store is required")
	}
	if cfg.Logger == nil {
		return nil, fmt.Errorf("worker: logger is required")
	}
	if cfg.WorkerID == "" {
		return nil, fmt.Errorf("worker: worker id is required")
	}
	if cfg.Concurrency < 1 {
		cfg.Concurrency = 1
	}
	if cfg.ClaimLimit < 1 {
		cfg.ClaimLimit = 1
	}
	if cfg.Lease <= 0 {
		cfg.Lease = 30 * time.Second
	}
	if cfg.Poll <= 0 {
		cfg.Poll = 500 * time.Millisecond
	}
	if cfg.Client == nil {
		cfg.Client = NewClient()
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	backoff := cfg.Backoff
	if len(backoff) == 0 {
		backoff = domain.DefaultBackoff
	}
	return &Pool{
		store:       cfg.Store,
		envelope:    cfg.Envelope,
		client:      cfg.Client,
		logger:      cfg.Logger,
		id:          cfg.WorkerID,
		concurrency: cfg.Concurrency,
		claimLimit:  cfg.ClaimLimit,
		lease:       cfg.Lease,
		poll:        cfg.Poll,
		backoff:     backoff,
		now:         cfg.Now,
		requestID:   newRequestID,
	}, nil
}

// Run starts the worker pool and blocks until ctx is cancelled.
func (p *Pool) Run(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)
	for range p.concurrency {
		g.Go(func() error { return p.loop(ctx) })
	}
	return g.Wait()
}

func (p *Pool) loop(ctx context.Context) error {
	for {
		if ctx.Err() != nil {
			return nil
		}
		jobs, err := p.store.ClaimDue(ctx, p.id, p.lease, p.claimLimit)
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			p.logger.Error("claim due", slog.String("worker_id", p.id), slog.Any("err", err))
			if !sleep(ctx, p.poll) {
				return nil
			}
			continue
		}
		if len(jobs) == 0 {
			if !sleep(ctx, p.poll) {
				return nil
			}
			continue
		}
		for _, job := range jobs {
			p.Process(context.WithoutCancel(ctx), job)
		}
	}
}

// Process decrypts, signs, POSTs, and records one claimed delivery.
// The claim transaction is already committed; this must not open a TX
// around the HTTP call.
func (p *Pool) Process(ctx context.Context, job domain.DeliveryClaim) {
	start := p.now()
	reqID := p.requestID()
	logger := p.logger.With(
		slog.String("worker_id", p.id),
		slog.String("delivery_id", job.Delivery.ID),
		slog.String("event_id", job.Delivery.EventID),
		slog.String("subscription_id", job.Delivery.SubscriptionID),
		slog.String("request_id", reqID),
	)

	statusCode, errMsg, postErr := p.send(ctx, job, reqID)
	duration := p.now().Sub(start)
	if duration < 0 {
		duration = 0
	}
	durationMs := int(duration / time.Millisecond)

	var next domain.Delivery
	var delay time.Duration
	success := postErr == nil && statusCode >= 200 && statusCode <= 299
	var recErr error
	if success {
		next, recErr = job.Delivery.RecordSuccess()
	} else {
		next, delay, recErr = job.Delivery.RecordFailure(p.backoff)
	}
	if recErr != nil {
		logger.Error("delivery transition", slog.Any("err", recErr))
		return
	}

	var codePtr *int
	if postErr == nil && statusCode > 0 {
		codePtr = &statusCode
	}
	var errPtr *string
	if errMsg != "" {
		errPtr = &errMsg
	}

	err := p.store.RecordAttempt(ctx, domain.AttemptRecord{
		DeliveryID:   next.ID,
		WorkerID:     p.id,
		Status:       next.Status,
		AttemptCount: next.AttemptCount,
		RetryDelay:   delay,
		StatusCode:   codePtr,
		ErrorMessage: errPtr,
		DurationMs:   durationMs,
		RequestID:    reqID,
	})
	if err != nil {
		if errors.Is(err, domain.ErrLeaseLost) {
			logger.Warn("lease lost", slog.Int("attempt", next.AttemptCount), slog.Any("err", err))
			return
		}
		logger.Error("record attempt", slog.Any("err", err))
		return
	}

	attrs := []any{
		slog.Int("attempt", next.AttemptCount),
		slog.String("outcome", string(next.Status)),
		slog.Int("duration_ms", durationMs),
	}
	if codePtr != nil {
		attrs = append(attrs, slog.Int("status_code", *codePtr))
	}
	logger.Info("delivery attempt finished", attrs...)
}

func (p *Pool) send(ctx context.Context, job domain.DeliveryClaim, reqID string) (status int, errMsg string, err error) {
	plain, err := p.envelope.Open(job.SigningSecretEnc)
	if err != nil {
		return 0, "decrypt signing secret", err
	}
	defer zero(plain)

	ts := p.now().UTC().Unix()
	sig := sign.Sign(plain, ts, job.Payload)

	hdr := make(http.Header)
	hdr.Set("Content-Type", "application/json")
	hdr.Set("User-Agent", UserAgent)
	hdr.Set(sign.HeaderIdempotencyKey, job.Delivery.EventID)
	hdr.Set(sign.HeaderTimestamp, strconv.FormatInt(ts, 10))
	hdr.Set(sign.HeaderSignature, sig)
	hdr.Set("X-Request-Id", reqID)
	for k, v := range job.Headers {
		if reservedOutboundHeader(k) {
			continue
		}
		hdr.Set(k, v)
	}

	attemptCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	status, err = postWebhook(attemptCtx, p.client, job.TargetURL, job.Payload, hdr)
	if err != nil {
		if isTimeout(err) {
			return 0, "http: request timeout", err
		}
		return 0, "http: request failed", err
	}
	if status < 200 || status > 299 {
		return status, fmt.Sprintf("http: status %d", status), nil
	}
	return status, "", nil
}

func reservedOutboundHeader(name string) bool {
	lower := strings.ToLower(name)
	switch lower {
	case "host", "content-length", "authorization", "user-agent", "content-type":
		return true
	}
	return strings.HasPrefix(lower, "x-courier-")
}

func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func newRequestID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
