package onboarding

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
)

const tokenBytes = 32

func NewToken() (string, []byte, error) {
	secret := make([]byte, tokenBytes)
	if _, err := rand.Read(secret); err != nil {
		return "", nil, err
	}
	raw := base64.RawURLEncoding.EncodeToString(secret)
	return raw, TokenDigest(raw), nil
}

func TokenDigest(raw string) []byte {
	digest := sha256.Sum256([]byte(raw))
	return digest[:]
}

func ValidateToken(raw string) error {
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil || len(decoded) != tokenBytes {
		return errors.New("malformed onboarding token")
	}
	return nil
}
