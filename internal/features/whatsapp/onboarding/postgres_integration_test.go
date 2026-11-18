//go:build integration

package onboarding

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	authmodels "github.com/harzeezco/zeexco_commerce/internal/features/authentication/models"
	"github.com/harzeezco/zeexco_commerce/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresOnboardingAtomicBoundary(t *testing.T) {
	ctx := context.Background()
	pool := onboardingTestPool(t, ctx)
	repository := NewRepository(pool)
	raw, digest, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	prepared, err := repository.PrepareWelcome(ctx, "verified-wa-id", "business-phone-id", digest, now, now.Add(30*time.Minute), now.Add(-time.Minute))
	if err != nil || !prepared.Send {
		t.Fatalf("PrepareWelcome()=%#v error=%v", prepared, err)
	}

	var stored []byte
	if err := pool.QueryRow(ctx, `SELECT token_hash FROM whatsapp_onboarding_sessions`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if string(stored) == raw || string(stored) != string(digest) {
		t.Fatal("database did not store only the token digest")
	}

	completion, err := repository.Complete(ctx, digest, integrationInput(""), now.Add(time.Minute))
	if err != nil {
		t.Fatalf("Complete() error=%v", err)
	}
	if completion.Session.Status != StatusFirstProductPending || completion.Session.LinkedUserID == nil || completion.Session.LinkedBusinessID == nil {
		t.Fatalf("session=%#v", completion.Session)
	}
	assertCount(t, ctx, pool, "users", 1)
	assertCount(t, ctx, pool, "businesses", 1)
	assertCount(t, ctx, pool, "business_memberships", 1)
	assertCount(t, ctx, pool, "whatsapp_identities", 1)
	assertCount(t, ctx, pool, "audit_events", 5)
	var storedEmail, storedPassword *string
	if err := pool.QueryRow(ctx, `SELECT email, password_hash FROM users WHERE id = $1`, completion.Registration.User.ID).Scan(&storedEmail, &storedPassword); err != nil {
		t.Fatal(err)
	}
	if storedEmail != nil || storedPassword != nil {
		t.Fatalf("WhatsApp-only credentials email=%v password_hash=%v, want NULL/NULL", storedEmail, storedPassword)
	}
	var auditsWithSession int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE metadata ? 'onboarding_session_id'`).Scan(&auditsWithSession); err != nil || auditsWithSession != 5 {
		t.Fatalf("audits with session metadata=%d error=%v", auditsWithSession, err)
	}
	var role, resolvedUser string
	if err := pool.QueryRow(ctx, `SELECT role FROM business_memberships`).Scan(&role); err != nil || role != "OWNER" {
		t.Fatalf("role=%q error=%v", role, err)
	}
	if err := pool.QueryRow(ctx, `SELECT user_id::text FROM whatsapp_identities WHERE provider='META_WHATSAPP' AND provider_user_id='verified-wa-id'`).Scan(&resolvedUser); err != nil || resolvedUser != completion.Registration.User.ID {
		t.Fatalf("resolved user=%q error=%v", resolvedUser, err)
	}
	if _, err := repository.Complete(ctx, digest, integrationInput("other@example.com"), now.Add(2*time.Minute)); !errors.Is(err, ErrSessionConsumed) {
		t.Fatalf("second completion error=%v", err)
	}
}

func TestPostgresOnboardingDuplicateEmailRollsBack(t *testing.T) {
	ctx := context.Background()
	pool := onboardingTestPool(t, ctx)
	if _, err := pool.Exec(ctx, `INSERT INTO users(first_name,last_name,email,password_hash,status) VALUES ('Existing','User','existing@example.com','hash','ACTIVE')`); err != nil {
		t.Fatal(err)
	}
	repository := NewRepository(pool)
	_, digest, _ := NewToken()
	now := time.Now().UTC()
	if _, err := repository.PrepareWelcome(ctx, "wa-duplicate-email", "phone-id", digest, now, now.Add(time.Hour), now.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Complete(ctx, digest, integrationInput("EXISTING@example.com"), now.Add(time.Minute)); !errors.Is(err, authmodels.ErrEmailAlreadyRegistered) {
		t.Fatalf("error=%v", err)
	}
	assertCount(t, ctx, pool, "users", 1)
	assertCount(t, ctx, pool, "businesses", 0)
	assertCount(t, ctx, pool, "business_memberships", 0)
	assertCount(t, ctx, pool, "whatsapp_identities", 0)
	var state string
	if err := pool.QueryRow(ctx, `SELECT status FROM whatsapp_onboarding_sessions`).Scan(&state); err != nil || state != StatusRegistrationPending {
		t.Fatalf("state=%q error=%v", state, err)
	}
}

func TestPostgresSimultaneousCompletionHasOneWinner(t *testing.T) {
	ctx := context.Background()
	pool := onboardingTestPool(t, ctx)
	repository := NewRepository(pool)
	_, digest, _ := NewToken()
	now := time.Now().UTC()
	if _, err := repository.PrepareWelcome(ctx, "wa-concurrent", "phone-id", digest, now, now.Add(time.Hour), now.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := repository.Complete(context.Background(), digest, integrationInput("concurrent@example.com"), now.Add(time.Minute))
			results <- err
		}()
	}
	wait.Wait()
	close(results)
	successes, consumed := 0, 0
	for err := range results {
		if err == nil {
			successes++
		} else if errors.Is(err, ErrSessionConsumed) {
			consumed++
		} else {
			t.Fatalf("unexpected error=%v", err)
		}
	}
	if successes != 1 || consumed != 1 {
		t.Fatalf("successes=%d consumed=%d", successes, consumed)
	}
	assertCount(t, ctx, pool, "users", 1)
	assertCount(t, ctx, pool, "businesses", 1)
}

func TestPostgresIdentityConflictDoesNotReassignOrPartiallyRegister(t *testing.T) {
	ctx := context.Background()
	pool := onboardingTestPool(t, ctx)
	var existingUserID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO users(first_name,last_name,email,password_hash,status)
		VALUES ('Existing','Owner','existing-owner@example.com','hash','ACTIVE')
		RETURNING id::text
	`).Scan(&existingUserID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO whatsapp_identities(user_id,provider,provider_user_id)
		VALUES ($1,'META_WHATSAPP','wa-conflict')
	`, existingUserID); err != nil {
		t.Fatal(err)
	}
	repository := NewRepository(pool)
	_, digest, _ := NewToken()
	now := time.Now().UTC()
	if _, err := repository.PrepareWelcome(ctx, "wa-conflict", "phone-id", digest, now, now.Add(time.Hour), now.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Complete(ctx, digest, integrationInput("new-owner@example.com"), now.Add(time.Minute)); !errors.Is(err, ErrIdentityConflict) {
		t.Fatalf("Complete() error=%v, want identity conflict", err)
	}
	assertCount(t, ctx, pool, "users", 1)
	assertCount(t, ctx, pool, "businesses", 0)
	assertCount(t, ctx, pool, "business_memberships", 0)
	assertCount(t, ctx, pool, "whatsapp_identities", 1)
	var linkedUserID, state string
	if err := pool.QueryRow(ctx, `SELECT user_id::text FROM whatsapp_identities WHERE provider_user_id='wa-conflict'`).Scan(&linkedUserID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM whatsapp_onboarding_sessions`).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if linkedUserID != existingUserID || state != StatusRegistrationPending {
		t.Fatalf("linked user=%q state=%q", linkedUserID, state)
	}
}

func TestPostgresExpiredTokenCannotCreateOrLinkAccount(t *testing.T) {
	ctx := context.Background()
	pool := onboardingTestPool(t, ctx)
	repository := NewRepository(pool)
	_, digest, _ := NewToken()
	now := time.Now().UTC()
	if _, err := repository.PrepareWelcome(ctx, "wa-expired", "phone-id", digest, now, now.Add(time.Minute), now.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Complete(ctx, digest, integrationInput("expired@example.com"), now.Add(2*time.Minute)); !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("Complete() error=%v, want expired", err)
	}
	assertCount(t, ctx, pool, "users", 0)
	assertCount(t, ctx, pool, "businesses", 0)
	assertCount(t, ctx, pool, "business_memberships", 0)
	assertCount(t, ctx, pool, "whatsapp_identities", 0)
	var state string
	if err := pool.QueryRow(ctx, `SELECT status FROM whatsapp_onboarding_sessions`).Scan(&state); err != nil || state != StatusExpired {
		t.Fatalf("state=%q error=%v", state, err)
	}
}

func integrationInput(email string) authmodels.RegistrationInput {
	return authmodels.RegistrationInput{FirstName: "Ada", LastName: "Lovelace", BusinessName: "Analytical Commerce", Email: strings.ToLower(email)}
}

func assertCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, table string, want int) {
	t.Helper()
	var got int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM "+pgx.Identifier{table}.Sanitize()).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("%s count=%d want=%d", table, got, want)
	}
}

func onboardingTestPool(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; start postgres-test and set the dedicated test URL")
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.ToLower(config.ConnConfig.Database), "test") {
		t.Fatalf("TEST_DATABASE_URL database %q must contain 'test'", config.ConnConfig.Database)
	}
	admin, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		t.Fatal(err)
	}
	schema := "zeexco_onboarding_it_" + hex.EncodeToString(random)
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(context.Background(), "DROP SCHEMA "+identifier+" CASCADE") })
	testConfig := config.Copy()
	testConfig.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, testConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := database.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	return pool
}
