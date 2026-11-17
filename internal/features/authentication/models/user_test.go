package models

import (
	"testing"
	"time"
)

func TestNewUserRequiresCanonicalUUID(t *testing.T) {
	now := time.Unix(1800000000, 0).UTC()
	if _, err := NewUser("usr_0123456789abcdef", "Ada", "Lovelace", "ada@example.com", UserStatusActive, now, now); err == nil {
		t.Fatal("NewUser() accepted retired usr_ identifier")
	}

	user, err := NewUser(
		"00000000-0000-4000-8000-000000000001",
		"Ada",
		"Lovelace",
		"ada@example.com",
		UserStatusActive,
		now,
		now,
	)
	if err != nil {
		t.Fatalf("NewUser() UUID error = %v", err)
	}
	if user.ID != "00000000-0000-4000-8000-000000000001" {
		t.Fatalf("user ID = %q", user.ID)
	}
}
