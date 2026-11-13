//go:build integration

package repositories

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	authmodels "github.com/harzeezco/zeexco_commerce/internal/features/authentication/models"
	"github.com/harzeezco/zeexco_commerce/internal/features/authentication/security"
	"github.com/harzeezco/zeexco_commerce/internal/features/authentication/services"
	businessauthorization "github.com/harzeezco/zeexco_commerce/internal/features/business/authorization"
	"github.com/harzeezco/zeexco_commerce/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresWebAuthenticationFoundation(t *testing.T) {
	t.Run("login persists only refresh digest", testPostgresLogin)
	t.Run("refresh rotates and old token fails", testPostgresRefreshRotation)
	t.Run("expired refresh fails closed", testPostgresExpiredRefresh)
	t.Run("logout revokes refresh session", testPostgresLogout)
	t.Run("business authorization isolates owners", testPostgresBusinessAuthorization)
	t.Run("concurrent refresh has one winner", testPostgresConcurrentRefresh)
}

func testPostgresExpiredRefresh(t *testing.T) {
	ctx := context.Background()
	pool := migratedRegistrationTestPool(t, ctx)
	registerIntegrationUser(t, ctx, pool, "expired-refresh@example.com")
	authService, _ := postgresAuthenticationService(t, pool)
	login, err := authService.Login(ctx, services.LoginCommand{
		Email: "expired-refresh@example.com", Password: integrationPassword,
	})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE refresh_sessions
		SET created_at = created_at - interval '2 days',
		    expires_at = created_at - interval '1 day'
	`); err != nil {
		t.Fatalf("expire refresh session: %v", err)
	}
	if _, err := authService.Refresh(ctx, login.RefreshToken); !errors.Is(err, services.ErrInvalidRefresh) {
		t.Fatalf("expired Refresh() error = %v", err)
	}
}

func testPostgresLogin(t *testing.T) {
	ctx := context.Background()
	pool := migratedRegistrationTestPool(t, ctx)
	registration := registerIntegrationUser(t, ctx, pool, "login@example.com")
	authService, tokens := postgresAuthenticationService(t, pool)
	result, err := authService.Login(ctx, services.LoginCommand{
		Email: " LOGIN@Example.com ", Password: integrationPassword,
	})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	principal, err := tokens.Verify(result.AccessToken)
	if err != nil || principal.UserID != registration.User.ID {
		t.Fatalf("access principal=%#v error=%v", principal, err)
	}
	var storedHash []byte
	if err := pool.QueryRow(ctx, `SELECT token_hash FROM refresh_sessions`).Scan(&storedHash); err != nil {
		t.Fatalf("load refresh hash: %v", err)
	}
	if string(storedHash) == result.RefreshToken || string(storedHash) != string(security.HashRefreshToken(result.RefreshToken)) {
		t.Fatal("database did not store only the refresh-token digest")
	}
	assertAuthenticationAuditCount(t, ctx, pool, "USER_LOGGED_IN", 1)
	assertAuthenticationAuditCount(t, ctx, pool, "REFRESH_SESSION_CREATED", 1)
}

func testPostgresRefreshRotation(t *testing.T) {
	ctx := context.Background()
	pool := migratedRegistrationTestPool(t, ctx)
	registerIntegrationUser(t, ctx, pool, "rotation@example.com")
	authService, _ := postgresAuthenticationService(t, pool)
	login, err := authService.Login(ctx, services.LoginCommand{Email: "rotation@example.com", Password: integrationPassword})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	rotated, err := authService.Refresh(ctx, login.RefreshToken)
	if err != nil {
		t.Fatalf("Refresh(A) error = %v", err)
	}
	if _, err := authService.Refresh(ctx, login.RefreshToken); !errors.Is(err, services.ErrInvalidRefresh) {
		t.Fatalf("reused Refresh(A) error = %v", err)
	}
	if _, err := authService.Refresh(ctx, rotated.RefreshToken); err != nil {
		t.Fatalf("Refresh(B) error = %v", err)
	}
	assertAuthenticationAuditCount(t, ctx, pool, "REFRESH_SESSION_ROTATED", 2)
}

func testPostgresLogout(t *testing.T) {
	ctx := context.Background()
	pool := migratedRegistrationTestPool(t, ctx)
	registerIntegrationUser(t, ctx, pool, "logout@example.com")
	authService, _ := postgresAuthenticationService(t, pool)
	login, err := authService.Login(ctx, services.LoginCommand{Email: "logout@example.com", Password: integrationPassword})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if err := authService.Logout(ctx, login.RefreshToken); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if _, err := authService.Refresh(ctx, login.RefreshToken); !errors.Is(err, services.ErrInvalidRefresh) {
		t.Fatalf("refresh after logout error = %v", err)
	}
	assertAuthenticationAuditCount(t, ctx, pool, "USER_LOGGED_OUT", 1)
}

func testPostgresBusinessAuthorization(t *testing.T) {
	ctx := context.Background()
	pool := migratedRegistrationTestPool(t, ctx)
	first := registerIntegrationUser(t, ctx, pool, "first@example.com")
	second := registerIntegrationUser(t, ctx, pool, "second@example.com")
	authorizer := businessauthorization.NewService(NewAuthenticationRepository(pool))
	principal := authmodels.Principal{UserID: first.User.ID}
	if _, err := authorizer.Authorize(ctx, principal, first.Business.ID); err != nil {
		t.Fatalf("authorize own business: %v", err)
	}
	if _, err := authorizer.Authorize(ctx, principal, second.Business.ID); !errors.Is(err, businessauthorization.ErrBusinessAccessDenied) {
		t.Fatalf("authorize another business error = %v", err)
	}
}

func testPostgresConcurrentRefresh(t *testing.T) {
	ctx := context.Background()
	pool := migratedRegistrationTestPool(t, ctx)
	registerIntegrationUser(t, ctx, pool, "refresh-concurrency@example.com")
	authService, _ := postgresAuthenticationService(t, pool)
	login, err := authService.Login(ctx, services.LoginCommand{Email: "refresh-concurrency@example.com", Password: integrationPassword})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}

	results := make(chan error, 2)
	var waitGroup sync.WaitGroup
	for range 2 {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			_, err := authService.Refresh(context.Background(), login.RefreshToken)
			results <- err
		}()
	}
	waitGroup.Wait()
	close(results)
	successes, rejected := 0, 0
	for err := range results {
		if err == nil {
			successes++
		} else if errors.Is(err, services.ErrInvalidRefresh) {
			rejected++
		} else {
			t.Fatalf("unexpected refresh error: %v", err)
		}
	}
	if successes != 1 || rejected != 1 {
		t.Fatalf("successes=%d rejected=%d", successes, rejected)
	}
	var active int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM refresh_sessions WHERE revoked_at IS NULL`).Scan(&active); err != nil {
		t.Fatalf("count active refresh sessions: %v", err)
	}
	if active != 1 {
		t.Fatalf("active refresh sessions = %d, want 1", active)
	}
}

func registerIntegrationUser(t *testing.T, ctx context.Context, pool *pgxpool.Pool, email string) *authmodels.Registration {
	t.Helper()
	registration, err := postgresRegisterService(t, pool, security.NewBcryptPasswordHasher()).Register(ctx, integrationRegisterCommand(email))
	if err != nil {
		t.Fatalf("register integration user: %v", err)
	}
	return registration
}

func postgresAuthenticationService(t *testing.T, pool *pgxpool.Pool) (*services.AuthenticationService, *security.AccessTokenManager) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate token keys: %v", err)
	}
	privateDER, _ := x509.MarshalPKCS8PrivateKey(privateKey)
	publicDER, _ := x509.MarshalPKIXPublicKey(publicKey)
	tokens, err := security.NewAccessTokenManager(
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER}),
		pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER}),
		"zeexco", "zeexco-api", 15*time.Minute,
	)
	if err != nil {
		t.Fatalf("NewAccessTokenManager() error = %v", err)
	}
	service, err := services.NewAuthenticationService(
		NewAuthenticationRepository(pool), security.NewBcryptPasswordHasher(), tokens, 30*24*time.Hour,
	)
	if err != nil {
		t.Fatalf("NewAuthenticationService() error = %v", err)
	}
	return service, tokens
}

func assertAuthenticationAuditCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventType string, want int) {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE event_type = $1`, eventType).Scan(&count); err != nil {
		t.Fatalf("count audit event %s: %v", eventType, err)
	}
	if count != want {
		t.Fatalf("audit event %s count=%d want=%d", eventType, count, want)
	}
}

func TestPostgresRegistrationFoundation(t *testing.T) {
	t.Run("successful atomic registration", testSuccessfulPostgresRegistration)
	t.Run("duplicate email has no orphans", testDuplicateEmailHasNoOrphans)
	t.Run("business failure rolls back user", testRegistrationRollback)
	t.Run("concurrent duplicate email has one winner", testConcurrentRegistration)
}

func testSuccessfulPostgresRegistration(t *testing.T) {
	ctx := context.Background()
	pool := migratedRegistrationTestPool(t, ctx)
	service := postgresRegisterService(t, pool, security.NewBcryptPasswordHasher())

	registration, err := service.Register(ctx, integrationRegisterCommand("owner@example.com"))
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if !authmodels.IsUUID(registration.User.ID) || !authmodels.IsUUID(registration.Business.ID) ||
		!authmodels.IsUUID(registration.Membership.ID) {
		t.Fatalf("registration IDs are not UUIDs: %#v", registration)
	}
	if registration.Membership.UserID != registration.User.ID ||
		registration.Membership.BusinessID != registration.Business.ID ||
		registration.Membership.Role != "OWNER" {
		t.Fatalf("owner membership = %#v", registration.Membership)
	}
	if registration.User.Status != authmodels.UserStatusPending {
		t.Fatalf("user status = %s", registration.User.Status)
	}

	assertTableCount(t, ctx, pool, "users", 1)
	assertTableCount(t, ctx, pool, "businesses", 1)
	assertTableCount(t, ctx, pool, "business_memberships", 1)
	assertTableCount(t, ctx, pool, "audit_events", 3)
	assertTableCount(t, ctx, pool, "email_verifications", 1)
	assertTableCount(t, ctx, pool, "email_delivery_jobs", 1)
	assertTableCount(t, ctx, pool, "whatsapp_identities", 0)

	var passwordHash string
	if err := pool.QueryRow(ctx, `SELECT password_hash FROM users WHERE id = $1`, registration.User.ID).Scan(&passwordHash); err != nil {
		t.Fatalf("load password hash: %v", err)
	}
	if passwordHash == integrationPassword {
		t.Fatal("plaintext password was stored")
	}
	if err := security.ComparePassword(passwordHash, integrationPassword); err != nil {
		t.Fatalf("stored password hash does not verify: %v", err)
	}

	var userRegistered, businessCreated, verificationRequested int
	if err := pool.QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE event_type = 'USER_REGISTERED'),
			count(*) FILTER (WHERE event_type = 'BUSINESS_CREATED'),
			count(*) FILTER (WHERE event_type = 'EMAIL_VERIFICATION_REQUESTED')
		FROM audit_events
	`).Scan(&userRegistered, &businessCreated, &verificationRequested); err != nil {
		t.Fatalf("load audit events: %v", err)
	}
	if userRegistered != 1 || businessCreated != 1 || verificationRequested != 1 {
		t.Fatalf("audit events USER_REGISTERED=%d BUSINESS_CREATED=%d EMAIL_VERIFICATION_REQUESTED=%d", userRegistered, businessCreated, verificationRequested)
	}
}

func testDuplicateEmailHasNoOrphans(t *testing.T) {
	ctx := context.Background()
	pool := migratedRegistrationTestPool(t, ctx)
	service := postgresRegisterService(t, pool, fixedPasswordHasher{})

	if _, err := service.Register(ctx, integrationRegisterCommand("duplicate@example.com")); err != nil {
		t.Fatalf("first Register() error = %v", err)
	}
	if _, err := service.Register(ctx, integrationRegisterCommand("DUPLICATE@example.com")); !errors.Is(err, authmodels.ErrEmailAlreadyRegistered) {
		t.Fatalf("second Register() error = %v, want duplicate email", err)
	}

	assertRegistrationCounts(t, ctx, pool, 1)
}

func testRegistrationRollback(t *testing.T) {
	ctx := context.Background()
	pool := migratedRegistrationTestPool(t, ctx)
	repository := NewRegistrationRepository(pool)

	_, err := repository.Register(ctx, authmodels.RegistrationInput{
		FirstName: "Ada", LastName: "Lovelace", Email: "rollback@example.com",
		PasswordHash: "test-only-password-hash", BusinessName: "",
	})
	if err == nil {
		t.Fatal("Register() error = nil, want business constraint failure")
	}
	assertRegistrationCounts(t, ctx, pool, 0)
}

func testConcurrentRegistration(t *testing.T) {
	ctx := context.Background()
	pool := migratedRegistrationTestPool(t, ctx)
	service := postgresRegisterService(t, pool, fixedPasswordHasher{})

	const attempts = 12
	errorsChannel := make(chan error, attempts)
	var waitGroup sync.WaitGroup
	for range attempts {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			_, err := service.Register(context.Background(), integrationRegisterCommand("concurrent@example.com"))
			errorsChannel <- err
		}()
	}
	waitGroup.Wait()
	close(errorsChannel)

	successes := 0
	duplicates := 0
	for err := range errorsChannel {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, authmodels.ErrEmailAlreadyRegistered):
			duplicates++
		default:
			t.Fatalf("unexpected concurrent registration error: %v", err)
		}
	}
	if successes != 1 || duplicates != attempts-1 {
		t.Fatalf("successes=%d duplicates=%d", successes, duplicates)
	}
	assertRegistrationCounts(t, ctx, pool, 1)
}

func postgresRegisterService(t *testing.T, pool *pgxpool.Pool, hasher services.PasswordHasher) *services.RegisterService {
	t.Helper()
	service, err := services.NewRegisterService(NewRegistrationRepository(pool), hasher)
	if err != nil {
		t.Fatalf("NewRegisterService() error = %v", err)
	}
	return service
}

const integrationPassword = "correct horse battery staple"

func integrationRegisterCommand(email string) services.RegisterCommand {
	return services.RegisterCommand{
		FirstName: "Ada", LastName: "Lovelace", BusinessName: "Analytical Commerce",
		Email: email, Password: integrationPassword,
	}
}

type fixedPasswordHasher struct{}

func (fixedPasswordHasher) Hash(string) (string, error) {
	return "test-only-password-hash", nil
}

func assertRegistrationCounts(t *testing.T, ctx context.Context, pool *pgxpool.Pool, accounts int) {
	t.Helper()
	assertTableCount(t, ctx, pool, "users", accounts)
	assertTableCount(t, ctx, pool, "businesses", accounts)
	assertTableCount(t, ctx, pool, "business_memberships", accounts)
	assertTableCount(t, ctx, pool, "audit_events", accounts*3)
	assertTableCount(t, ctx, pool, "email_verifications", accounts)
	assertTableCount(t, ctx, pool, "email_delivery_jobs", accounts)
}

func assertTableCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, table string, want int) {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM "+pgx.Identifier{table}.Sanitize()).Scan(&count); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	if count != want {
		t.Fatalf("%s count = %d, want %d", table, count, want)
	}
}

func migratedRegistrationTestPool(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; start postgres-test and set the dedicated test URL")
	}

	adminConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	if !strings.Contains(strings.ToLower(adminConfig.ConnConfig.Database), "test") {
		t.Fatalf("TEST_DATABASE_URL database %q must contain 'test'", adminConfig.ConnConfig.Database)
	}
	adminPool, err := pgxpool.NewWithConfig(ctx, adminConfig)
	if err != nil {
		t.Fatalf("connect to integration PostgreSQL: %v", err)
	}
	t.Cleanup(adminPool.Close)

	schema := "zeexco_registration_it_" + randomRegistrationHex(t, 8)
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err := adminPool.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
		t.Fatalf("create integration schema: %v", err)
	}
	t.Cleanup(func() { _, _ = adminPool.Exec(context.Background(), "DROP SCHEMA "+identifier+" CASCADE") })

	testConfig := adminConfig.Copy()
	testConfig.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, testConfig)
	if err != nil {
		t.Fatalf("connect to integration schema: %v", err)
	}
	t.Cleanup(pool.Close)
	applyRegistrationMigrations(t, ctx, pool)
	return pool
}

func applyRegistrationMigrations(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := database.Migrate(ctx, pool); err != nil {
		t.Fatalf("apply project migrations: %v", err)
	}
}

func randomRegistrationHex(t *testing.T, byteCount int) string {
	t.Helper()
	value := make([]byte, byteCount)
	if _, err := rand.Read(value); err != nil {
		t.Fatalf("generate integration schema suffix: %v", err)
	}
	return hex.EncodeToString(value)
}
