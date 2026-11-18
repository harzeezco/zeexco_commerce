package services

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/harzeezco/zeexco_commerce/internal/features/authentication/models"
)

func TestBrowserRegistrationHashesTrustedOnboardingToken(t *testing.T) {
	raw := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))
	repository := &fakeRegistrationRepository{registration: testRegistration()}
	hasher := &fakePasswordHasher{hash: "password-hash"}
	command := validRegisterCommand()
	command.OnboardingToken = raw
	if _, err := newTestRegisterService(t, repository, hasher).Register(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	want := sha256.Sum256([]byte(raw))
	if !bytes.Equal(repository.input.OnboardingTokenHash, want[:]) || repository.input.PasswordHash != "password-hash" || hasher.password != command.Password {
		t.Fatal("browser registration did not preserve credentials and digest its onboarding token")
	}
}

func TestMalformedOnboardingTokenCannotReachPersistence(t *testing.T) {
	repository := &fakeRegistrationRepository{}
	hasher := &fakePasswordHasher{hash: "unused"}
	command := validRegisterCommand()
	command.OnboardingToken = "malformed-token"
	_, err := newTestRegisterService(t, repository, hasher).Register(context.Background(), command)
	if !errors.Is(err, models.ErrOnboardingInvalid) || repository.calls != 0 || hasher.calls != 0 {
		t.Fatal("malformed token reached hashing or persistence")
	}
}
