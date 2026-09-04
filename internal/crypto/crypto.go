// Package crypto encrypts chat message content at rest with AES-256-GCM.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

const prefix = "enc:v1:"

// Cipher seals and opens message content. A nil *Cipher passes values through
// unchanged, which is also how legacy plaintext rows stay readable.
type Cipher struct {
	gcm cipher.AEAD
}

// NewCipher derives a 32-byte key from secret: a 64-char hex string is used
// as-is, any other secret is hashed with SHA-256. An empty secret disables
// encryption and returns a nil cipher.
func NewCipher(secret string) (*Cipher, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil, nil
	}
	key, err := decodeKey(secret)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{gcm: gcm}, nil
}

func decodeKey(secret string) ([]byte, error) {
	if len(secret) == 64 {
		if key, err := hex.DecodeString(secret); err == nil {
			return key, nil
		}
	}
	sum := sha256.Sum256([]byte(secret))
	return sum[:], nil
}

// Encrypt seals plaintext as "enc:v1:<base64(nonce|ciphertext)>". Empty input
// stays empty so blank rows remain blank.
func (c *Cipher) Encrypt(plaintext string) (string, error) {
	if c == nil || plaintext == "" {
		return plaintext, nil
	}
	nonce := make([]byte, c.gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("encrypt message: %w", err)
	}
	sealed := c.gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return prefix + base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt opens "enc:v1:..." values. Values without the prefix were written
// before encryption was enabled and pass through unchanged.
func (c *Cipher) Decrypt(value string) (string, error) {
	if c == nil || !strings.HasPrefix(value, prefix) {
		return value, nil
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, prefix))
	if err != nil {
		return "", fmt.Errorf("decrypt message: %w", err)
	}
	if len(raw) < c.gcm.NonceSize() {
		return "", errors.New("decrypt message: ciphertext too short")
	}
	nonce, sealed := raw[:c.gcm.NonceSize()], raw[c.gcm.NonceSize():]
	plaintext, err := c.gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt message: %w", err)
	}
	return string(plaintext), nil
}
