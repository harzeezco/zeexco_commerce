package emailverification

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"

	authmodels "github.com/harzeezco/zeexco_commerce/internal/features/authentication/models"
)

var ErrInvalidToken = errors.New("email verification token is invalid")

type TokenClaims struct {
	VerificationID string
	ExpiresAt      time.Time
}

type TokenManager struct {
	secret []byte
	now    func() time.Time
}

func NewTokenManager(secret []byte) (*TokenManager, error) {
	if len(secret) < 32 {
		return nil, errors.New("email verification HMAC secret must be at least 32 bytes")
	}
	return &TokenManager{secret: append([]byte(nil), secret...), now: time.Now}, nil
}

func (m *TokenManager) Mint(verificationID string, expiresAt time.Time) (string, error) {
	if m == nil || !authmodels.IsUUID(verificationID) || !expiresAt.After(m.now().UTC()) {
		return "", ErrInvalidToken
	}
	payload := verificationID + "|" + strconv.FormatInt(expiresAt.UTC().Unix(), 10)
	encoded := base64.RawURLEncoding.EncodeToString([]byte(payload))
	signature := m.sign("v1." + encoded)
	return "v1." + encoded + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func (m *TokenManager) Verify(raw string) (TokenClaims, error) {
	if m == nil {
		return TokenClaims{}, ErrInvalidToken
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 3 || parts[0] != "v1" {
		return TokenClaims{}, ErrInvalidToken
	}
	received, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(received, m.sign(parts[0]+"."+parts[1])) {
		return TokenClaims{}, ErrInvalidToken
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return TokenClaims{}, ErrInvalidToken
	}
	values := strings.Split(string(payload), "|")
	if len(values) != 2 || !authmodels.IsUUID(values[0]) {
		return TokenClaims{}, ErrInvalidToken
	}
	expiresUnix, err := strconv.ParseInt(values[1], 10, 64)
	if err != nil {
		return TokenClaims{}, ErrInvalidToken
	}
	expiresAt := time.Unix(expiresUnix, 0).UTC()
	return TokenClaims{VerificationID: values[0], ExpiresAt: expiresAt}, nil
}

func (m *TokenManager) sign(value string) []byte {
	mac := hmac.New(sha256.New, m.secret)
	_, _ = mac.Write([]byte(value))
	return mac.Sum(nil)
}
