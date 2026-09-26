package domain

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base32"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"
)

// API key format: "co_" + prefix + "_" + secret, where prefix and secret are
// lowercase unpadded base32 of random bytes (see ADR 0004).
const (
	apiKeyScheme      = "co"
	apiKeyPrefixBytes = 5  // 8 base32 chars
	apiKeySecretBytes = 32 // 52 base32 chars

	// APIKeyPrefixLen is the length of the prefix segment.
	APIKeyPrefixLen = 8
	// APIKeyHashLen is the length in bytes of a stored API key hash.
	APIKeyHashLen = sha256.Size
	// APIKeyNameMaxLen bounds the operator-supplied key name.
	APIKeyNameMaxLen = 100

	apiKeySecretLen = 52
)

var apiKeyEncoding = base32.NewEncoding("abcdefghijklmnopqrstuvwxyz234567").WithPadding(base32.NoPadding)

// ErrMalformedAPIKey means a presented credential is not shaped like a
// courier API key. It unwraps to ErrValidation.
var ErrMalformedAPIKey = fmt.Errorf("%w: malformed api key", ErrValidation)

// APIKey is a producer credential as stored. The raw key is never stored.
type APIKey struct {
	ID        string
	Name      string
	Prefix    string
	Hash      []byte
	CreatedAt time.Time
	RevokedAt *time.Time
}

// Revoked reports whether the key may no longer authenticate.
func (k APIKey) Revoked() bool { return k.RevokedAt != nil }

// GeneratedAPIKey is a freshly minted key. Raw must be shown to the operator
// once and then discarded; only Prefix and Hash are persisted.
type GeneratedAPIKey struct {
	Raw    string
	Prefix string
	Hash   []byte
}

// APIKeyHasher hashes and verifies API keys with HMAC-SHA256 keyed by a
// server-side pepper.
type APIKeyHasher struct {
	pepper []byte
}

// NewAPIKeyHasher returns a hasher for the given pepper. The pepper must be
// at least minLen bytes; callers pass the configured minimum.
func NewAPIKeyHasher(pepper []byte, minLen int) (APIKeyHasher, error) {
	if len(pepper) < minLen {
		return APIKeyHasher{}, &ValidationError{Field: "pepper", Reason: fmt.Sprintf("must be at least %d bytes", minLen)}
	}
	return APIKeyHasher{pepper: append([]byte(nil), pepper...)}, nil
}

// Hash returns HMAC-SHA256(pepper, raw).
func (h APIKeyHasher) Hash(raw string) []byte {
	mac := hmac.New(sha256.New, h.pepper)
	mac.Write([]byte(raw))
	return mac.Sum(nil)
}

// Verify reports, in constant time, whether raw hashes to want.
func (h APIKeyHasher) Verify(raw string, want []byte) bool {
	return hmac.Equal(h.Hash(raw), want)
}

// Generate mints a new API key using entropy from rand (crypto/rand.Reader in
// production).
func (h APIKeyHasher) Generate(rand io.Reader) (GeneratedAPIKey, error) {
	buf := make([]byte, apiKeyPrefixBytes+apiKeySecretBytes)
	if _, err := io.ReadFull(rand, buf); err != nil {
		return GeneratedAPIKey{}, fmt.Errorf("read entropy: %w", err)
	}
	prefix := apiKeyEncoding.EncodeToString(buf[:apiKeyPrefixBytes])
	secret := apiKeyEncoding.EncodeToString(buf[apiKeyPrefixBytes:])
	raw := apiKeyScheme + "_" + prefix + "_" + secret
	return GeneratedAPIKey{Raw: raw, Prefix: prefix, Hash: h.Hash(raw)}, nil
}

// ParseAPIKeyPrefix validates the shape of a presented key and returns its
// lookup prefix. It does not prove the key is genuine; use Verify for that.
func ParseAPIKeyPrefix(raw string) (string, error) {
	scheme, rest, ok := strings.Cut(raw, "_")
	if !ok || scheme != apiKeyScheme {
		return "", ErrMalformedAPIKey
	}
	prefix, secret, ok := strings.Cut(rest, "_")
	if !ok || len(prefix) != APIKeyPrefixLen || len(secret) != apiKeySecretLen {
		return "", ErrMalformedAPIKey
	}
	if !isBase32Lower(prefix) || !isBase32Lower(secret) {
		return "", ErrMalformedAPIKey
	}
	return prefix, nil
}

// ValidateAPIKeyName checks an operator-supplied key name.
func ValidateAPIKeyName(name string) error {
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		return &ValidationError{Field: "name", Reason: "is required"}
	case utf8.RuneCountInString(name) > APIKeyNameMaxLen:
		return &ValidationError{Field: "name", Reason: fmt.Sprintf("must be at most %d characters", APIKeyNameMaxLen)}
	}
	return nil
}

func isBase32Lower(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < 'a' || c > 'z') && (c < '2' || c > '7') {
			return false
		}
	}
	return true
}
