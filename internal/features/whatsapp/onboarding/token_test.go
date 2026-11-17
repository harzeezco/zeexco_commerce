package onboarding

import (
	"bytes"
	"testing"
)

func TestTokenIsHighEntropyOpaqueAndStoredAsDigest(t *testing.T) {
	rawA, digestA, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	rawB, digestB, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateToken(rawA); err != nil {
		t.Fatalf("ValidateToken() error = %v", err)
	}
	if rawA == rawB || bytes.Equal(digestA, digestB) {
		t.Fatal("independent tokens must differ")
	}
	if len(digestA) != 32 || bytes.Contains(digestA, []byte(rawA)) {
		t.Fatal("token storage value must be a 32-byte one-way digest")
	}
	for _, malformed := range []string{"", "phone=234", rawA + "x", "***"} {
		if ValidateToken(malformed) == nil {
			t.Fatalf("ValidateToken(%q) unexpectedly succeeded", malformed)
		}
	}
}
