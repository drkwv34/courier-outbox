package domain

import (
	"crypto/aes"
	"crypto/cipher"
	"fmt"
	"io"
)

// Envelope seals and opens small secrets with AES-256-GCM. The stored form is
// nonce concatenated with ciphertext.
type Envelope struct {
	aead cipher.AEAD
}

// NewEnvelope returns an envelope keyed by a 32-byte AES key. The key is
// copied by the cipher; callers may zero their buffer afterwards.
func NewEnvelope(key []byte) (Envelope, error) {
	if len(key) != 32 {
		return Envelope{}, &ValidationError{Field: "encryption_key", Reason: "must be 32 bytes"}
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return Envelope{}, fmt.Errorf("aes cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return Envelope{}, fmt.Errorf("aes gcm: %w", err)
	}
	return Envelope{aead: aead}, nil
}

// Seal encrypts plaintext using a random nonce from rand.
func (e Envelope) Seal(rand io.Reader, plaintext []byte) ([]byte, error) {
	nonce := make([]byte, e.aead.NonceSize())
	if _, err := io.ReadFull(rand, nonce); err != nil {
		return nil, fmt.Errorf("read nonce: %w", err)
	}
	return e.aead.Seal(nonce, nonce, plaintext, nil), nil
}

// Open decrypts a blob produced by Seal. Failures do not distinguish
// "wrong key" from "corrupt ciphertext".
func (e Envelope) Open(blob []byte) ([]byte, error) {
	n := e.aead.NonceSize()
	if len(blob) < n {
		return nil, fmt.Errorf("ciphertext truncated")
	}
	nonce, ct := blob[:n], blob[n:]
	plain, err := e.aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	return plain, nil
}
