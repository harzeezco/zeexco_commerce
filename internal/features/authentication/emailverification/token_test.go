package emailverification

import (
	"errors"
	"testing"
	"time"
)

func TestTokenManagerSignsAndRejectsTampering(t *testing.T) {
	manager, err := NewTokenManager([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1800000000, 0).UTC()
	manager.now = func() time.Time { return now }
	token, err := manager.Mint("00000000-0000-4000-8000-000000000001", now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	claims, err := manager.Verify(token)
	if err != nil || claims.VerificationID != "00000000-0000-4000-8000-000000000001" {
		t.Fatalf("claims=%#v error=%v", claims, err)
	}
	if _, err := manager.Verify(token + "x"); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("tampered token error=%v", err)
	}
}
