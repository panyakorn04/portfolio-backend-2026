package security

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestCredentialCipherRoundTripAndTamperDetection(t *testing.T) {
	t.Parallel()

	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	cipher, err := NewCredentialCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := cipher.Encrypt(map[string]string{"token": "secret", "name": "X-API-Key"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(encrypted, "secret") {
		t.Fatal("ciphertext exposed plaintext")
	}
	decrypted, err := cipher.Decrypt(encrypted)
	if err != nil {
		t.Fatal(err)
	}
	if decrypted["token"] != "secret" || decrypted["name"] != "X-API-Key" {
		t.Fatalf("unexpected plaintext: %#v", decrypted)
	}
	if _, err := cipher.Decrypt(encrypted + "tampered"); err == nil {
		t.Fatal("tampered ciphertext must fail")
	}
	scoped, err := cipher.EncryptFor("credential-1:bearer:v1", map[string]string{"token": "secret"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cipher.DecryptFor("credential-2:bearer:v1", scoped); err == nil {
		t.Fatal("ciphertext must be bound to its credential scope")
	}
}

func TestCredentialCipherRejectsInvalidKeys(t *testing.T) {
	t.Parallel()

	for _, key := range []string{"", "not-base64", base64.StdEncoding.EncodeToString([]byte("too-short"))} {
		if _, err := NewCredentialCipher(key); err == nil {
			t.Fatalf("expected key %q to fail", key)
		}
	}
}
