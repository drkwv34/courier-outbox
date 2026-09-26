package domain

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"testing/iotest"
)

const testPepper = "0123456789abcdef0123456789abcdef"

// Golden vector computed independently (Python base64 + hmac) from entropy
// bytes 0x00..0x24 and testPepper.
const (
	goldenRaw     = "co_aaaqeaye_audaocajbifqydiob4ibceqtcqkrmfyydenbwha5dypsaijcemsa"
	goldenPrefix  = "aaaqeaye"
	goldenHashHex = "141ac44763d4946d08c507f05118fb8af042cc336ff763248f6d90fc91faeb8e"
)

func mustHasher(t *testing.T, pepper string) APIKeyHasher {
	t.Helper()
	h, err := NewAPIKeyHasher([]byte(pepper), 32)
	if err != nil {
		t.Fatalf("NewAPIKeyHasher: %v", err)
	}
	return h
}

func counterEntropy() *bytes.Reader {
	b := make([]byte, 37)
	for i := range b {
		b[i] = byte(i)
	}
	return bytes.NewReader(b)
}

func TestAPIKeyHasher_GenerateGolden(t *testing.T) {
	t.Parallel()

	got, err := mustHasher(t, testPepper).Generate(counterEntropy())
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got.Raw != goldenRaw {
		t.Fatalf("Raw = %q, want %q", got.Raw, goldenRaw)
	}
	if got.Prefix != goldenPrefix {
		t.Fatalf("Prefix = %q, want %q", got.Prefix, goldenPrefix)
	}
	if hex.EncodeToString(got.Hash) != goldenHashHex {
		t.Fatalf("Hash = %x, want %s", got.Hash, goldenHashHex)
	}
}

func TestAPIKeyHasher_GenerateRoundTrip(t *testing.T) {
	t.Parallel()

	h := mustHasher(t, testPepper)
	seen := map[string]bool{}
	for range 64 {
		k, err := h.Generate(rand.Reader)
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if seen[k.Raw] {
			t.Fatalf("duplicate key generated: %s", k.Prefix)
		}
		seen[k.Raw] = true

		prefix, err := ParseAPIKeyPrefix(k.Raw)
		if err != nil {
			t.Fatalf("ParseAPIKeyPrefix(generated) error: %v", err)
		}
		if prefix != k.Prefix {
			t.Fatalf("parsed prefix %q, want %q", prefix, k.Prefix)
		}
		if len(k.Hash) != APIKeyHashLen {
			t.Fatalf("hash length = %d, want %d", len(k.Hash), APIKeyHashLen)
		}
		if !h.Verify(k.Raw, k.Hash) {
			t.Fatal("Verify(generated) = false, want true")
		}
		if strings.Contains(hex.EncodeToString(k.Hash), k.Raw) {
			t.Fatal("hash must not embed the raw key")
		}
	}
}

func TestAPIKeyHasher_GenerateEntropyFailure(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	_, err := mustHasher(t, testPepper).Generate(iotest.ErrReader(boom))
	if !errors.Is(err, boom) {
		t.Fatalf("Generate error = %v, want wrapped entropy error", err)
	}

	_, err = mustHasher(t, testPepper).Generate(bytes.NewReader(make([]byte, 10)))
	if err == nil {
		t.Fatal("Generate with short entropy succeeded, want error")
	}
}

func TestAPIKeyHasher_Verify(t *testing.T) {
	t.Parallel()

	h := mustHasher(t, testPepper)
	hash := h.Hash(goldenRaw)
	tampered := goldenRaw[:len(goldenRaw)-1] + "b"

	tests := []struct {
		name string
		raw  string
		hash []byte
		want bool
	}{
		{name: "match", raw: goldenRaw, hash: hash, want: true},
		{name: "wrong secret", raw: tampered, hash: hash, want: false},
		{name: "empty raw", raw: "", hash: hash, want: false},
		{name: "empty hash", raw: goldenRaw, hash: nil, want: false},
		{name: "truncated hash", raw: goldenRaw, hash: hash[:16], want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := h.Verify(tt.raw, tt.hash); got != tt.want {
				t.Fatalf("Verify() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAPIKeyHasher_PepperChangesHash(t *testing.T) {
	t.Parallel()

	a := mustHasher(t, testPepper)
	b := mustHasher(t, strings.ToUpper(testPepper))
	if bytes.Equal(a.Hash(goldenRaw), b.Hash(goldenRaw)) {
		t.Fatal("different peppers produced the same hash")
	}
	if b.Verify(goldenRaw, a.Hash(goldenRaw)) {
		t.Fatal("hash verified under a different pepper")
	}
}

func TestAPIKeyHasher_HashIsDeterministic(t *testing.T) {
	t.Parallel()

	h := mustHasher(t, testPepper)
	if !bytes.Equal(h.Hash(goldenRaw), h.Hash(goldenRaw)) {
		t.Fatal("Hash is not deterministic")
	}
}

func TestNewAPIKeyHasher_RejectsShortPepper(t *testing.T) {
	t.Parallel()

	_, err := NewAPIKeyHasher([]byte(testPepper[:31]), 32)
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("NewAPIKeyHasher(short) error = %v, want ErrValidation", err)
	}
}

func TestNewAPIKeyHasher_CopiesPepper(t *testing.T) {
	t.Parallel()

	pepper := []byte(testPepper)
	h, err := NewAPIKeyHasher(pepper, 32)
	if err != nil {
		t.Fatal(err)
	}
	before := h.Hash(goldenRaw)
	pepper[0] ^= 0xff
	if !bytes.Equal(before, h.Hash(goldenRaw)) {
		t.Fatal("mutating the caller's pepper changed the hasher")
	}
}

func TestParseAPIKeyPrefix(t *testing.T) {
	t.Parallel()

	secret := goldenRaw[len("co_aaaqeaye_"):]
	tests := []struct {
		name       string
		raw        string
		wantPrefix string
		wantErr    bool
	}{
		{name: "valid", raw: goldenRaw, wantPrefix: goldenPrefix},
		{name: "empty", raw: "", wantErr: true},
		{name: "wrong scheme", raw: "sk_aaaqeaye_" + secret, wantErr: true},
		{name: "no separators", raw: "coaaaqeaye" + secret, wantErr: true},
		{name: "missing secret", raw: "co_aaaqeaye", wantErr: true},
		{name: "short prefix", raw: "co_aaaqeay_" + secret, wantErr: true},
		{name: "long prefix", raw: "co_aaaqeayea_" + secret, wantErr: true},
		{name: "short secret", raw: "co_aaaqeaye_" + secret[1:], wantErr: true},
		{name: "uppercase", raw: strings.ToUpper(goldenRaw), wantErr: true},
		{name: "invalid base32 digit", raw: "co_aaaqeay1_" + secret, wantErr: true},
		{name: "extra segment", raw: goldenRaw + "_x", wantErr: true},
		{name: "surrounding space", raw: " " + goldenRaw, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseAPIKeyPrefix(tt.raw)
			if tt.wantErr {
				if !errors.Is(err, ErrMalformedAPIKey) || !errors.Is(err, ErrValidation) {
					t.Fatalf("ParseAPIKeyPrefix(%q) error = %v, want ErrMalformedAPIKey", tt.raw, err)
				}
				return
			}
			if err != nil || got != tt.wantPrefix {
				t.Fatalf("ParseAPIKeyPrefix(%q) = %q, %v; want %q", tt.raw, got, err, tt.wantPrefix)
			}
		})
	}
}

func TestValidateAPIKeyName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{name: "simple", in: "ci"},
		{name: "max length", in: strings.Repeat("é", APIKeyNameMaxLen)},
		{name: "empty", in: "", wantErr: true},
		{name: "blank", in: "   ", wantErr: true},
		{name: "too long", in: strings.Repeat("a", APIKeyNameMaxLen+1), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateAPIKeyName(tt.in)
			if tt.wantErr != (err != nil) {
				t.Fatalf("ValidateAPIKeyName(%q) = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			var ve *ValidationError
			if tt.wantErr && (!errors.As(err, &ve) || ve.Field != "name") {
				t.Fatalf("error = %v, want *ValidationError on field name", err)
			}
		})
	}
}
