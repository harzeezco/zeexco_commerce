package security

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testUserID = "00000000-0000-4000-8000-000000000001"

func TestAccessTokenManagerMintsAndVerifiesStrictClaims(t *testing.T) {
	manager := testAccessTokenManager(t, "zeexco", "zeexco-api")
	raw, err := manager.Mint(testUserID)
	if err != nil {
		t.Fatalf("Mint() error = %v", err)
	}
	principal, err := manager.Verify(raw)
	if err != nil || principal.UserID != testUserID {
		t.Fatalf("Verify() principal=%#v error=%v", principal, err)
	}
}

func TestAccessTokenVerifierNeedsOnlyPublicKey(t *testing.T) {
	manager := testAccessTokenManager(t, "zeexco", "zeexco-api")
	raw, err := manager.Mint(testUserID)
	if err != nil {
		t.Fatalf("Mint() error = %v", err)
	}
	publicDER, err := x509.MarshalPKIXPublicKey(manager.verifier.publicKey)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	verifier, err := NewAccessTokenVerifier(
		pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER}),
		"zeexco", "zeexco-api",
	)
	if err != nil {
		t.Fatalf("NewAccessTokenVerifier() error = %v", err)
	}
	verifier.now = manager.now
	principal, err := verifier.Verify(raw)
	if err != nil || principal.UserID != testUserID {
		t.Fatalf("Verify() principal=%#v error=%v", principal, err)
	}
}

func TestAccessTokenManagerRejectsExpiredToken(t *testing.T) {
	manager := testAccessTokenManager(t, "zeexco", "zeexco-api")
	issuedAt := time.Unix(1800000000, 0).UTC()
	manager.now = func() time.Time { return issuedAt }
	raw, err := manager.Mint(testUserID)
	if err != nil {
		t.Fatalf("Mint() error = %v", err)
	}
	manager.now = func() time.Time { return issuedAt.Add(16 * time.Minute) }
	if _, err := manager.Verify(raw); !errors.Is(err, ErrInvalidAccessToken) {
		t.Fatalf("Verify() error = %v, want invalid token", err)
	}
}

func TestAccessTokenManagerRejectsWrongIssuerAudienceSignatureAndAlgorithm(t *testing.T) {
	manager := testAccessTokenManager(t, "zeexco", "zeexco-api")
	raw, err := manager.Mint(testUserID)
	if err != nil {
		t.Fatalf("Mint() error = %v", err)
	}

	wrongIssuer := *manager
	wrongIssuerVerifier := *manager.verifier
	wrongIssuerVerifier.issuer = "other-issuer"
	wrongIssuer.verifier = &wrongIssuerVerifier
	wrongAudience := *manager
	wrongAudienceVerifier := *manager.verifier
	wrongAudienceVerifier.audience = "other-audience"
	wrongAudience.verifier = &wrongAudienceVerifier
	wrongSignature := testAccessTokenManager(t, "zeexco", "zeexco-api")

	for name, verifier := range map[string]*AccessTokenManager{
		"issuer": &wrongIssuer, "audience": &wrongAudience, "signature": wrongSignature,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := verifier.Verify(raw); !errors.Is(err, ErrInvalidAccessToken) {
				t.Fatalf("Verify() error = %v", err)
			}
		})
	}

	hsToken := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Issuer: "zeexco", Subject: testUserID, Audience: jwt.ClaimStrings{"zeexco-api"},
		IssuedAt:  jwt.NewNumericDate(manager.now()),
		ExpiresAt: jwt.NewNumericDate(manager.now().Add(15 * time.Minute)),
		ID:        "00000000-0000-4000-8000-000000000002",
	})
	hsRaw, err := hsToken.SignedString([]byte("not-an-ed25519-key"))
	if err != nil {
		t.Fatalf("sign HS token: %v", err)
	}
	if _, err := manager.Verify(hsRaw); !errors.Is(err, ErrInvalidAccessToken) {
		t.Fatalf("wrong algorithm error = %v", err)
	}
}

func TestAccessTokenManagerRejectsMalformedSubject(t *testing.T) {
	manager := testAccessTokenManager(t, "zeexco", "zeexco-api")
	now := manager.now().UTC()
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, accessTokenClaims{RegisteredClaims: jwt.RegisteredClaims{
		Issuer: "zeexco", Subject: "not-a-uuid", Audience: jwt.ClaimStrings{"zeexco-api"},
		IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
		ID: "00000000-0000-4000-8000-000000000002",
	}})
	raw, err := token.SignedString(manager.privateKey)
	if err != nil {
		t.Fatalf("sign malformed subject token: %v", err)
	}
	if _, err := manager.Verify(raw); !errors.Is(err, ErrInvalidAccessToken) {
		t.Fatalf("Verify() error = %v", err)
	}
}

func TestRefreshTokensAreRandomOpaqueAndHashed(t *testing.T) {
	first, firstHash, err := NewRefreshToken()
	if err != nil {
		t.Fatalf("NewRefreshToken() error = %v", err)
	}
	second, secondHash, err := NewRefreshToken()
	if err != nil {
		t.Fatalf("NewRefreshToken() second error = %v", err)
	}
	if first == second || string(firstHash) == string(secondHash) {
		t.Fatal("refresh tokens are not unique")
	}
	if first == string(firstHash) || len(firstHash) != 32 || len(first) < 40 {
		t.Fatalf("raw length=%d hash length=%d", len(first), len(firstHash))
	}
	if string(HashRefreshToken(first)) != string(firstHash) {
		t.Fatal("refresh hash is not deterministic")
	}
}

func testAccessTokenManager(t *testing.T, issuer, audience string) *AccessTokenManager {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate Ed25519 key: %v", err)
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}
	publicDER, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	manager, err := NewAccessTokenManager(
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER}),
		pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER}),
		issuer, audience, 15*time.Minute,
	)
	if err != nil {
		t.Fatalf("NewAccessTokenManager() error = %v", err)
	}
	manager.now = func() time.Time { return time.Unix(1800000000, 0).UTC() }
	return manager
}
