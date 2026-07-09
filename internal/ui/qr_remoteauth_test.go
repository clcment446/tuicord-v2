package ui

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"testing"
)

// TestNonceProof verifies the remote-auth nonce_proof reply is the base64url
// SHA-256 of the decrypted nonce (Discord rejects the raw nonce), which is what
// lets the flow advance from nonce_proof to the QR fingerprint.
func TestNonceProof(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	nonce := []byte("the-quick-brown-fox-jumps-over-the-lazy-dog")
	encrypted, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, &key.PublicKey, nonce, nil)
	if err != nil {
		t.Fatal(err)
	}

	got, err := nonceProof(key, base64.StdEncoding.EncodeToString(encrypted))
	if err != nil {
		t.Fatal(err)
	}

	sum := sha256.Sum256(nonce)
	want := base64.RawURLEncoding.EncodeToString(sum[:])
	if got != want {
		t.Fatalf("nonceProof = %q, want base64url(sha256(nonce)) = %q", got, want)
	}
	// Guard against the earlier bug of echoing the raw nonce back.
	if got == base64.RawURLEncoding.EncodeToString(nonce) {
		t.Fatal("nonceProof returned the raw nonce; it must return its SHA-256 hash")
	}
}

// TestNonceProofRejectsBadBase64 checks the decode error path.
func TestNonceProofRejectsBadBase64(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := nonceProof(key, "not valid base64!!"); err == nil {
		t.Fatal("nonceProof accepted invalid base64")
	}
}
