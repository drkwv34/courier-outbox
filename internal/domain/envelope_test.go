package domain

import (
	"bytes"
	"crypto/rand"
	"errors"
	"testing"
	"testing/iotest"
)

func testEnvelope(t *testing.T) Envelope {
	t.Helper()
	e, err := NewEnvelope([]byte("local-dev-only-enc-key-32bytes!!"))
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestEnvelope_RoundTrip(t *testing.T) {
	t.Parallel()

	e := testEnvelope(t)
	plain := []byte("webhook-signing-secret-32-bytes!")
	blob, err := e.Seal(rand.Reader, plain)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(blob, plain) {
		t.Fatal("ciphertext embeds plaintext")
	}
	got, err := e.Open(blob)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("Open = %q, want %q", got, plain)
	}
}

func TestEnvelope_WrongKey(t *testing.T) {
	t.Parallel()

	e := testEnvelope(t)
	blob, err := e.Seal(rand.Reader, []byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	other, err := NewEnvelope([]byte("other-dev-only-enc-key-32bytes!!"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Open(blob); err == nil {
		t.Fatal("Open with wrong key succeeded")
	}
}

func TestEnvelope_Truncated(t *testing.T) {
	t.Parallel()

	e := testEnvelope(t)
	if _, err := e.Open([]byte("short")); err == nil {
		t.Fatal("Open(truncated) succeeded")
	}
}

func TestEnvelope_NonceEntropyFailure(t *testing.T) {
	t.Parallel()

	e := testEnvelope(t)
	boom := errors.New("boom")
	if _, err := e.Seal(iotest.ErrReader(boom), []byte("x")); !errors.Is(err, boom) {
		t.Fatalf("Seal error = %v, want wrapped entropy error", err)
	}
}

func TestNewEnvelope_RejectsShortKey(t *testing.T) {
	t.Parallel()

	_, err := NewEnvelope([]byte("short"))
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("NewEnvelope(short) = %v, want ErrValidation", err)
	}
}

func TestGenerateSigningSecret(t *testing.T) {
	t.Parallel()

	got, err := GenerateSigningSecret(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != SigningSecretBytes {
		t.Fatalf("len = %d, want %d", len(got), SigningSecretBytes)
	}
	other, err := GenerateSigningSecret(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(got, other) {
		t.Fatal("GenerateSigningSecret produced identical secrets")
	}

	boom := errors.New("boom")
	if _, err := GenerateSigningSecret(iotest.ErrReader(boom)); !errors.Is(err, boom) {
		t.Fatalf("GenerateSigningSecret error = %v, want wrapped entropy error", err)
	}
}
