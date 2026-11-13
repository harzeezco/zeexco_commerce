package security

import (
	"strings"
	"testing"
)

func TestBcryptPasswordHasherDoesNotStorePlaintext(t *testing.T) {
	const password = "correct horse battery staple"
	hash, err := NewBcryptPasswordHasher().Hash(password)
	if err != nil {
		t.Fatalf("Hash() error = %v", err)
	}
	if hash == password || strings.Contains(hash, password) {
		t.Fatal("password hash contains plaintext password")
	}
	if err := ComparePassword(hash, password); err != nil {
		t.Fatalf("ComparePassword() correct password error = %v", err)
	}
	if err := ComparePassword(hash, "incorrect password"); err == nil {
		t.Fatal("ComparePassword() accepted incorrect password")
	}
}
