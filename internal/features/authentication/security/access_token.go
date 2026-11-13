package security

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/harzeezco/zeexco_commerce/internal/features/authentication/models"
)

const AccessTokenAlgorithm = "EdDSA"

var (
	ErrInvalidAccessToken = errors.New("access token is invalid")
	ErrPrivateKeyRequired = errors.New("JWT private signing key is required")
	ErrPublicKeyRequired  = errors.New("JWT public verification key is required")
)

type AccessTokenManager struct {
	privateKey ed25519.PrivateKey
	verifier   *AccessTokenVerifier
	lifetime   time.Duration
	now        func() time.Time
}

// AccessTokenVerifier is the public-key-only authentication boundary future
// services can use without receiving the private credential used to mint JWTs.
type AccessTokenVerifier struct {
	publicKey ed25519.PublicKey
	issuer    string
	audience  string
	now       func() time.Time
}

type accessTokenClaims struct {
	jwt.RegisteredClaims
}

func NewAccessTokenManager(
	privateKeyPEM []byte,
	publicKeyPEM []byte,
	issuer string,
	audience string,
	lifetime time.Duration,
) (*AccessTokenManager, error) {
	privateKey, err := parseEd25519PrivateKey(privateKeyPEM)
	if err != nil {
		return nil, err
	}
	publicKey, err := parseEd25519PublicKey(publicKeyPEM)
	if err != nil {
		return nil, err
	}
	if !privateKey.Public().(ed25519.PublicKey).Equal(publicKey) {
		return nil, errors.New("JWT public key does not match private signing key")
	}
	if issuer == "" || audience == "" || lifetime <= 0 {
		return nil, errors.New("JWT issuer, audience, and positive lifetime are required")
	}
	return &AccessTokenManager{
		privateKey: privateKey,
		verifier: &AccessTokenVerifier{
			publicKey: publicKey, issuer: issuer, audience: audience, now: time.Now,
		},
		lifetime: lifetime,
		now:      time.Now,
	}, nil
}

func NewAccessTokenVerifier(
	publicKeyPEM []byte,
	issuer string,
	audience string,
) (*AccessTokenVerifier, error) {
	publicKey, err := parseEd25519PublicKey(publicKeyPEM)
	if err != nil {
		return nil, err
	}
	if issuer == "" || audience == "" {
		return nil, errors.New("JWT issuer and audience are required")
	}
	return &AccessTokenVerifier{
		publicKey: publicKey, issuer: issuer, audience: audience, now: time.Now,
	}, nil
}

func (m *AccessTokenManager) Mint(userID string) (string, error) {
	if !isCanonicalUUID(userID) {
		return "", errors.New("access token subject must be a user UUID")
	}
	now := m.now().UTC()
	tokenID, err := newUUIDv4()
	if err != nil {
		return "", fmt.Errorf("generate access token ID: %w", err)
	}
	claims := accessTokenClaims{RegisteredClaims: jwt.RegisteredClaims{
		Issuer: m.verifier.issuer, Subject: userID, Audience: jwt.ClaimStrings{m.verifier.audience},
		ExpiresAt: jwt.NewNumericDate(now.Add(m.lifetime)),
		IssuedAt:  jwt.NewNumericDate(now), ID: tokenID,
	}}
	return jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims).SignedString(m.privateKey)
}

func (m *AccessTokenManager) Verify(raw string) (models.Principal, error) {
	return verifyAccessToken(raw, m.verifier.publicKey, m.verifier.issuer, m.verifier.audience, m.now)
}

func (v *AccessTokenVerifier) Verify(raw string) (models.Principal, error) {
	return verifyAccessToken(raw, v.publicKey, v.issuer, v.audience, v.now)
}

func verifyAccessToken(
	raw string,
	publicKey ed25519.PublicKey,
	issuer string,
	audience string,
	now func() time.Time,
) (models.Principal, error) {
	claims := &accessTokenClaims{}
	token, err := jwt.ParseWithClaims(
		raw,
		claims,
		func(token *jwt.Token) (any, error) {
			if token.Method.Alg() != AccessTokenAlgorithm {
				return nil, ErrInvalidAccessToken
			}
			return publicKey, nil
		},
		jwt.WithValidMethods([]string{AccessTokenAlgorithm}),
		jwt.WithIssuer(issuer),
		jwt.WithAudience(audience),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithTimeFunc(func() time.Time { return now().UTC() }),
	)
	if err != nil || token == nil || !token.Valid || !isCanonicalUUID(claims.Subject) || !isCanonicalUUID(claims.ID) {
		return models.Principal{}, ErrInvalidAccessToken
	}
	return models.Principal{UserID: claims.Subject}, nil
}

func isCanonicalUUID(value string) bool {
	return models.IsUUID(value) && value == strings.ToLower(value)
}

func (m *AccessTokenManager) Lifetime() time.Duration {
	return m.lifetime
}

func parseEd25519PrivateKey(value []byte) (ed25519.PrivateKey, error) {
	if len(value) == 0 {
		return nil, ErrPrivateKeyRequired
	}
	block, _ := pem.Decode(value)
	if block == nil {
		return nil, errors.New("JWT private key is not valid PEM")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse JWT private key: %w", err)
	}
	privateKey, ok := parsed.(ed25519.PrivateKey)
	if !ok {
		return nil, errors.New("JWT private key must be Ed25519 PKCS#8")
	}
	return privateKey, nil
}

func parseEd25519PublicKey(value []byte) (ed25519.PublicKey, error) {
	if len(value) == 0 {
		return nil, ErrPublicKeyRequired
	}
	block, _ := pem.Decode(value)
	if block == nil {
		return nil, errors.New("JWT public key is not valid PEM")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse JWT public key: %w", err)
	}
	publicKey, ok := parsed.(ed25519.PublicKey)
	if !ok {
		return nil, errors.New("JWT public key must be Ed25519 PKIX")
	}
	return publicKey, nil
}

func newUUIDv4() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(value[:])
	return fmt.Sprintf("%s-%s-%s-%s-%s", encoded[:8], encoded[8:12], encoded[12:16], encoded[16:20], encoded[20:]), nil
}
