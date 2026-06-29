package auth

import (
	"encoding/hex"
	"testing"

	"golang.org/x/crypto/scrypt"
)

func TestVerifyPasswordBcrypt(t *testing.T) {
	hash, err := HashPassword("s3cret-password")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}

	if !VerifyPassword("s3cret-password", hash) {
		t.Error("correct password should verify")
	}
	if VerifyPassword("wrong", hash) {
		t.Error("wrong password should not verify")
	}
}

// TestVerifyPasswordLegacyScrypt builds a hash using the same parameters as
// Node's crypto.scryptSync (the original backend) and verifies it round-trips.
func TestVerifyPasswordLegacyScrypt(t *testing.T) {
	salt := []byte("0123456789abcdef") // 16 bytes, as the original used
	derived, err := scrypt.Key([]byte("legacy-pass"), salt,
		legacyScryptN, legacyScryptR, legacyScryptP, legacyScryptKeyLen)
	if err != nil {
		t.Fatalf("scrypt: %v", err)
	}

	hash := hex.EncodeToString(salt) + ":" + hex.EncodeToString(derived)

	if !VerifyPassword("legacy-pass", hash) {
		t.Error("correct legacy password should verify")
	}
	if VerifyPassword("wrong", hash) {
		t.Error("wrong legacy password should not verify")
	}
}

func TestVerifyPasswordRejectsGarbage(t *testing.T) {
	if VerifyPassword("x", "not-a-valid-hash") {
		t.Error("garbage hash should not verify")
	}
}
