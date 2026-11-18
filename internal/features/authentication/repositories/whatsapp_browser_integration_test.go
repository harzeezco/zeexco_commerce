//go:build integration

package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/harzeezco/zeexco_commerce/internal/features/authentication/emailverification"
	authmodels "github.com/harzeezco/zeexco_commerce/internal/features/authentication/models"
	"github.com/harzeezco/zeexco_commerce/internal/features/authentication/security"
	"github.com/harzeezco/zeexco_commerce/internal/features/authentication/services"
	"github.com/harzeezco/zeexco_commerce/internal/features/whatsapp/onboarding"
)

func TestWhatsAppBrowserRegistrationActivatesAndLinksAtomically(t *testing.T) {
	ctx := context.Background()
	pool := migratedRegistrationTestPool(t, ctx)
	now := time.Now().UTC()
	raw, digest, err := onboarding.NewToken()
	if err != nil {
		t.Fatal(err)
	}
	repository := onboarding.NewRepository(pool)
	prepared, err := repository.PrepareWelcome(ctx, "15550009999", "test-phone-id", digest, now, now.Add(30*time.Minute), now.Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	command := integrationRegisterCommand("whatsapp-browser@example.com")
	command.OnboardingToken = raw
	service := postgresRegisterService(t, pool, security.NewBcryptPasswordHasher())
	registration, err := service.Register(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	if registration.User.Status != authmodels.UserStatusPending {
		t.Fatal("WhatsApp browser account bypassed email verification")
	}
	assertTableCount(t, ctx, pool, "whatsapp_identities", 0)
	assertEmailJobTypeCount(t, ctx, pool, emailverification.JobTypeEmailVerification, 1)
	var status, linkedUser string
	if err := pool.QueryRow(ctx, `SELECT status, linked_user_id::text FROM whatsapp_onboarding_sessions WHERE id=$1`, prepared.Session.ID).Scan(&status, &linkedUser); err != nil {
		t.Fatal(err)
	}
	if status != onboarding.StatusEmailVerificationPending || linkedUser != registration.User.ID {
		t.Fatal("onboarding source was not reserved for the pending account")
	}
	if _, err := service.Register(ctx, command); !errors.Is(err, authmodels.ErrOnboardingConsumed) {
		t.Fatalf("token replay error=%v", err)
	}
	assertTableCount(t, ctx, pool, "users", 1)
	// Another inbound message must not create a second signup during activation.
	_, retryDigest, _ := onboarding.NewToken()
	retry, err := repository.PrepareWelcome(ctx, "15550009999", "test-phone-id", retryDigest, now, now.Add(30*time.Minute), now.Add(-time.Minute))
	if err != nil || retry.Send {
		t.Fatalf("pending activation retriggered signup: %v", err)
	}
	verificationID, _ := loadVerification(t, ctx, pool, registration.User.ID)
	continued, err := emailverification.NewRepository(pool).VerifyWithContinuation(ctx, verificationID, time.Now().UTC())
	if err != nil || !continued {
		t.Fatalf("WhatsApp activation continuation=%v error=%v", continued, err)
	}
	assertTableCount(t, ctx, pool, "whatsapp_identities", 1)
	assertEmailJobTypeCount(t, ctx, pool, emailverification.JobTypeWelcome, 1)
	var provider, providerUserID, actualUser string
	if err := pool.QueryRow(ctx, `SELECT provider,provider_user_id,user_id::text FROM whatsapp_identities`).Scan(&provider, &providerUserID, &actualUser); err != nil {
		t.Fatal(err)
	}
	if provider != onboarding.ProviderMetaWhatsApp || providerUserID != "15550009999" || actualUser != registration.User.ID {
		t.Fatal("verified identity did not come from the trusted onboarding session")
	}
	if _, err := emailverification.NewRepository(pool).VerifyWithContinuation(ctx, verificationID, time.Now().UTC()); !errors.Is(err, emailverification.ErrVerificationConsumed) {
		t.Fatalf("replayed activation error=%v", err)
	}
	authentication, _ := postgresAuthenticationService(t, pool)
	if _, err := authentication.Login(ctx, services.LoginCommand{Email: command.Email, Password: command.Password}); err != nil {
		t.Fatalf("verified WhatsApp registrant cannot use web login: %v", err)
	}
	completion, claimed, err := repository.ClaimCompletionForUser(ctx, registration.User.ID, time.Now().UTC(), now.Add(-time.Minute))
	if err != nil || !claimed || completion.Status != onboarding.StatusFirstProductPending {
		t.Fatalf("first product continuation unavailable: %v", err)
	}
}

func TestWhatsAppBrowserRejectsUnknownAndExpiredTokensWithoutOrphans(t *testing.T) {
	ctx := context.Background()
	pool := migratedRegistrationTestPool(t, ctx)
	service := postgresRegisterService(t, pool, security.NewBcryptPasswordHasher())
	raw, digest, _ := onboarding.NewToken()
	command := integrationRegisterCommand("untrusted-source@example.com")
	command.OnboardingToken = raw
	if _, err := service.Register(ctx, command); !errors.Is(err, authmodels.ErrOnboardingInvalid) {
		t.Fatalf("unknown token error=%v", err)
	}
	now := time.Now().UTC()
	prepared, err := onboarding.NewRepository(pool).PrepareWelcome(ctx, "15550009998", "test-phone-id", digest, now, now.Add(30*time.Minute), now.Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE whatsapp_onboarding_sessions SET expires_at=created_at + interval '1 microsecond' WHERE id=$1`, prepared.Session.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Register(ctx, command); !errors.Is(err, authmodels.ErrOnboardingExpired) {
		t.Fatalf("expired token error=%v", err)
	}
	for _, table := range []string{"users", "businesses", "business_memberships", "email_verifications", "email_delivery_jobs", "whatsapp_identities"} {
		assertTableCount(t, ctx, pool, table, 0)
	}
}
