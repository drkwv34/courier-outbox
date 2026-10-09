// Package sign implements the v1 webhook signature scheme: HMAC-SHA256
// over "{timestamp}.{raw_body}" with header X-Courier-Signature: v1=<hex>.
package sign

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	// HeaderIdempotencyKey is the consumer dedupe key (event id).
	HeaderIdempotencyKey = "X-Courier-Idempotency-Key"
	// HeaderTimestamp is unix seconds of the signed payload.
	HeaderTimestamp = "X-Courier-Timestamp"
	// HeaderSignature is v1=<hex hmac>.
	HeaderSignature = "X-Courier-Signature"
	// VersionPrefix is the scheme version in the signature header.
	VersionPrefix = "v1="
	// MaxSkew is the maximum |now - timestamp| a verifier accepts.
	MaxSkew = 5 * time.Minute
)

// ErrInvalidSignature means the header does not match the payload.
var ErrInvalidSignature = errors.New("invalid signature")

// ErrTimestampSkew means the timestamp is more than MaxSkew from now.
var ErrTimestampSkew = errors.New("timestamp skew")

// CanonicalString is the bytes HMAC'd for v1: "{unix_seconds}.{raw_body}".
func CanonicalString(ts int64, body []byte) []byte {
	prefix := strconv.FormatInt(ts, 10) + "."
	out := make([]byte, len(prefix)+len(body))
	copy(out, prefix)
	copy(out[len(prefix):], body)
	return out
}

// Sign returns "v1=" plus the hex HMAC-SHA256 of the canonical string.
func Sign(secret []byte, ts int64, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(CanonicalString(ts, body))
	return VersionPrefix + hex.EncodeToString(mac.Sum(nil))
}

// Verify checks a v1 signature header against secret, timestamp, and body.
// now is used only for the 5-minute skew window.
func Verify(secret []byte, ts int64, body []byte, header string, now time.Time) error {
	if !strings.HasPrefix(header, VersionPrefix) {
		return fmt.Errorf("%w: missing v1 prefix", ErrInvalidSignature)
	}
	want, err := hex.DecodeString(strings.TrimPrefix(header, VersionPrefix))
	if err != nil {
		return fmt.Errorf("%w: malformed hex", ErrInvalidSignature)
	}
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(CanonicalString(ts, body))
	if !hmac.Equal(want, mac.Sum(nil)) {
		return ErrInvalidSignature
	}
	t := time.Unix(ts, 0)
	skew := now.Sub(t)
	if skew < 0 {
		skew = -skew
	}
	if skew > MaxSkew {
		return ErrTimestampSkew
	}
	return nil
}
