package sign

import (
	"errors"
	"testing"
	"time"
)

const goldenSecret = "whsec_test_secret_for_golden__"
const goldenTS = int64(1700000000)
const goldenBody = `{"hello":"world"}`
const goldenSig = "v1=181e21208e31fa0e50255e086cf32834280b38ee68b6eea9ed91c5aad7013f8e"

func TestSign_Golden(t *testing.T) {
	t.Parallel()
	got := Sign([]byte(goldenSecret), goldenTS, []byte(goldenBody))
	if got != goldenSig {
		t.Fatalf("Sign = %q, want %q", got, goldenSig)
	}
}

func TestVerify_Golden(t *testing.T) {
	t.Parallel()
	now := time.Unix(goldenTS, 0)
	if err := Verify([]byte(goldenSecret), goldenTS, []byte(goldenBody), goldenSig, now); err != nil {
		t.Fatal(err)
	}
}

func TestVerify_RejectsWrongBody(t *testing.T) {
	t.Parallel()
	now := time.Unix(goldenTS, 0)
	if err := Verify([]byte(goldenSecret), goldenTS, []byte(`{"hello":"nope"}`), goldenSig, now); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("err = %v, want ErrInvalidSignature", err)
	}
}

func TestVerify_RejectsSkew(t *testing.T) {
	t.Parallel()
	now := time.Unix(goldenTS, 0).Add(MaxSkew + time.Second)
	if err := Verify([]byte(goldenSecret), goldenTS, []byte(goldenBody), goldenSig, now); !errors.Is(err, ErrTimestampSkew) {
		t.Fatalf("err = %v, want ErrTimestampSkew", err)
	}
}

func TestVerify_AcceptsSkewBoundary(t *testing.T) {
	t.Parallel()
	now := time.Unix(goldenTS, 0).Add(MaxSkew)
	if err := Verify([]byte(goldenSecret), goldenTS, []byte(goldenBody), goldenSig, now); err != nil {
		t.Fatal(err)
	}
}

func TestVerify_MalformedHeader(t *testing.T) {
	t.Parallel()
	now := time.Unix(goldenTS, 0)
	if err := Verify([]byte(goldenSecret), goldenTS, []byte(goldenBody), "v2=ab", now); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("err = %v", err)
	}
}

func FuzzSignVerify(f *testing.F) {
	f.Add([]byte("secret"), int64(1700000000), []byte(`{"a":1}`))
	f.Fuzz(func(t *testing.T, secret []byte, ts int64, body []byte) {
		if len(secret) == 0 {
			return
		}
		hdr := Sign(secret, ts, body)
		now := time.Unix(ts, 0)
		if err := Verify(secret, ts, body, hdr, now); err != nil {
			t.Fatalf("round-trip: %v", err)
		}
	})
}
