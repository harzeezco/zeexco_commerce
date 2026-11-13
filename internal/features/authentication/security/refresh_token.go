package security

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

const refreshTokenBytes = 32

func NewRefreshToken() (raw string, hash []byte, err error) {
	secret := make([]byte, refreshTokenBytes)
	if _, err := rand.Read(secret); err != nil {
		return "", nil, err
	}
	raw = base64.RawURLEncoding.EncodeToString(secret)
	return raw, HashRefreshToken(raw), nil
}

func HashRefreshToken(raw string) []byte {
	digest := sha256.Sum256([]byte(raw))
	return digest[:]
}
