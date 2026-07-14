package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const credentialCipherVersion = "v1:"

type CredentialCipher struct {
	aead cipher.AEAD
}

func NewCredentialCipher(encodedKey string) (*CredentialCipher, error) {
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encodedKey))
	if err != nil || len(key) != 32 {
		return nil, errors.New("Studio credential encryption key must be a base64-encoded 32-byte key")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create credential cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create credential AEAD: %w", err)
	}
	return &CredentialCipher{aead: aead}, nil
}

func (c *CredentialCipher) Encrypt(values map[string]string) (string, error) {
	return c.EncryptFor("generic", values)
}

func (c *CredentialCipher) EncryptFor(scope string, values map[string]string) (string, error) {
	if c == nil || c.aead == nil {
		return "", errors.New("credential encryption is not configured")
	}
	if strings.TrimSpace(scope) == "" {
		return "", errors.New("credential encryption scope is required")
	}
	plaintext, err := json.Marshal(values)
	if err != nil {
		return "", fmt.Errorf("encode credential: %w", err)
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate credential nonce: %w", err)
	}
	aad := []byte(credentialCipherVersion + scope)
	sealed := c.aead.Seal(nil, nonce, plaintext, aad)
	payload := append(nonce, sealed...)
	return credentialCipherVersion + base64.RawURLEncoding.EncodeToString(payload), nil
}

func (c *CredentialCipher) Decrypt(encoded string) (map[string]string, error) {
	return c.DecryptFor("generic", encoded)
}

func (c *CredentialCipher) DecryptFor(scope, encoded string) (map[string]string, error) {
	if c == nil || c.aead == nil {
		return nil, errors.New("credential encryption is not configured")
	}
	if strings.TrimSpace(scope) == "" {
		return nil, errors.New("credential encryption scope is required")
	}
	if !strings.HasPrefix(encoded, credentialCipherVersion) {
		return nil, errors.New("credential ciphertext version is unsupported")
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(encoded, credentialCipherVersion))
	if err != nil || len(payload) <= c.aead.NonceSize() {
		return nil, errors.New("credential ciphertext is invalid")
	}
	nonce, ciphertext := payload[:c.aead.NonceSize()], payload[c.aead.NonceSize():]
	aad := []byte(credentialCipherVersion + scope)
	plaintext, err := c.aead.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, errors.New("credential ciphertext authentication failed")
	}
	values := map[string]string{}
	if err := json.Unmarshal(plaintext, &values); err != nil {
		return nil, errors.New("credential plaintext is invalid")
	}
	return values, nil
}
