package worker

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"time"
)

const (
	// UserAgent identifies courier on outbound webhook POSTs.
	UserAgent       = "courier-outbox/1.0"
	maxResponseBody = 8 << 10
	dialTimeout     = 2 * time.Second
	tlsTimeout      = 2 * time.Second
	requestTimeout  = 5 * time.Second
	maxIdlePerHost  = 4
)

// NewClient returns the isolated outbound webhook client (FR-DEL-003).
func NewClient() *http.Client {
	dialer := &net.Dialer{Timeout: dialTimeout}
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   tlsTimeout,
		ResponseHeaderTimeout: requestTimeout,
		MaxIdleConnsPerHost:   maxIdlePerHost,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
	}
	return &http.Client{
		Transport: transport,
		Timeout:   requestTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// postWebhook POSTs body to targetURL. It reads at most 8 KiB of the
// response, drains the rest, and closes the body. Redirects are not followed.
func postWebhook(ctx context.Context, client *http.Client, targetURL string, body []byte, hdr http.Header) (status int, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header = hdr.Clone()
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBody))
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}

func isTimeout(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}
