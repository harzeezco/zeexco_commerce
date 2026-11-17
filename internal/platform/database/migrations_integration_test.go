//go:build integration

package database

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"slices"
	"strings"
	"testing"

	migrationassets "github.com/harzeezco/zeexco_commerce/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestMigrateEmptyDatabaseToCurrentSchema(t *testing.T) {
	ctx := context.Background()
	pool := migrationTestPool(t, ctx)
	report, err := Migrate(ctx, pool)
	if err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	wantApplied := []string{
		"000001_create_users",
		"000003_create_whatsapp_identities",
		"000004_create_whatsapp_messages",
		"000005_add_whatsapp_identity_resolution_state",
		"000006_create_business_registration_foundation",
		"000007_create_refresh_sessions",
		"000008_create_whatsapp_onboarding_sessions",
		"000009_create_whatsapp_webhook_events",
		"000010_allow_active_users_without_password",
	}
	if !slices.Equal(report.Applied, wantApplied) || len(report.Baselined) != 0 {
		t.Fatalf("migration report = %#v", report)
	}
	assertCurrentSchema(t, ctx, pool)
	var version int
	var dirty bool
	if err := pool.QueryRow(ctx, `SELECT version, dirty FROM schema_migrations`).Scan(&version, &dirty); err != nil {
		t.Fatalf("read schema migration version: %v", err)
	}
	if version != 10 || dirty {
		t.Fatalf("tracked migration version=%d dirty=%v, want 10 clean", version, dirty)
	}

	second, err := Migrate(ctx, pool)
	if err != nil || len(second.Applied) != 0 || len(second.Baselined) != 0 {
		t.Fatalf("second Migrate() report=%#v error=%v", second, err)
	}
}

func TestMigrateLegacyPre000006SchemaWithoutDeletingUsers(t *testing.T) {
	ctx := context.Background()
	pool := migrationTestPool(t, ctx)
	applyLegacyPre000006(t, ctx, pool)
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (first_name, last_name, email, status)
		VALUES ('Legacy', 'Merchant', 'legacy@example.com', 'ACTIVE')
	`); err != nil {
		t.Fatalf("insert legacy user: %v", err)
	}

	report, err := Migrate(ctx, pool)
	if err != nil {
		t.Fatalf("Migrate() legacy error = %v", err)
	}
	wantBaselined := []string{
		"000001_create_users",
		"000003_create_whatsapp_identities",
		"000004_create_whatsapp_messages",
		"000005_add_whatsapp_identity_resolution_state",
	}
	wantApplied := []string{
		"000006_create_business_registration_foundation",
		"000007_create_refresh_sessions",
		"000008_create_whatsapp_onboarding_sessions",
		"000009_create_whatsapp_webhook_events",
		"000010_allow_active_users_without_password",
	}
	if !slices.Equal(report.Baselined, wantBaselined) ||
		!slices.Equal(report.Applied, wantApplied) ||
		report.LegacyActiveUsersWithoutPassword != 1 {
		t.Fatalf("legacy migration report = %#v", report)
	}
	assertCurrentSchema(t, ctx, pool)
	var passwordHash *string
	if err := pool.QueryRow(ctx, `SELECT password_hash FROM users WHERE email = 'legacy@example.com'`).Scan(&passwordHash); err != nil {
		t.Fatalf("load legacy password hash: %v", err)
	}
	if passwordHash != nil {
		t.Fatal("migration invented a password for a legacy user")
	}
	var passwordConstraintExists bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM pg_constraint
		WHERE conrelid = 'users'::regclass AND conname = 'users_active_password_hash_check')
	`).Scan(&passwordConstraintExists); err != nil {
		t.Fatalf("inspect password constraint: %v", err)
	}
	if passwordConstraintExists {
		t.Fatal("current schema still rejects legitimate WhatsApp-only ACTIVE users")
	}
}

func TestMigrateBlocksCaseInsensitiveDuplicateEmailsSafely(t *testing.T) {
	ctx := context.Background()
	pool := migrationTestPool(t, ctx)
	applyLegacyPre000006(t, ctx, pool)
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (first_name, last_name, email)
		VALUES ('First', 'Merchant', 'duplicate@example.com'),
		       ('Second', 'Merchant', 'DUPLICATE@example.com')
	`); err != nil {
		t.Fatalf("insert duplicate legacy emails: %v", err)
	}

	_, err := Migrate(ctx, pool)
	if err == nil || !strings.Contains(err.Error(), "1 case-insensitive duplicate email group") {
		t.Fatalf("Migrate() duplicate error = %v", err)
	}
	var users int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&users); err != nil {
		t.Fatalf("count preserved users: %v", err)
	}
	if users != 2 {
		t.Fatalf("users after blocked migration = %d, want 2", users)
	}
	exists, err := columnExistsFromPool(ctx, pool, "users", "password_hash")
	if err != nil || exists {
		t.Fatalf("password_hash exists=%v error=%v after blocked migration", exists, err)
	}
}

func migrationTestPool(t *testing.T, ctx context.Context) *pgxpool.Pool {
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

	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		t.Fatalf("generate schema suffix: %v", err)
	}
	schema := "zeexco_migration_it_" + hex.EncodeToString(random)
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err := adminPool.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
		t.Fatalf("create test schema: %v", err)
	}
	t.Cleanup(func() { _, _ = adminPool.Exec(context.Background(), "DROP SCHEMA "+identifier+" CASCADE") })

	testConfig := adminConfig.Copy()
	testConfig.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, testConfig)
	if err != nil {
		t.Fatalf("connect to migration test schema: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func applyLegacyPre000006(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	for _, file := range []string{
		"000001_create_users.up.sql",
		"000003_create_whatsapp_identities.up.sql",
		"000004_create_whatsapp_messages.up.sql",
		"000005_add_whatsapp_identity_resolution_state.up.sql",
	} {
		contents, err := migrationassets.Files.ReadFile(file)
		if err != nil {
			t.Fatalf("read legacy migration %s: %v", file, err)
		}
		if _, err := pool.Exec(ctx, string(contents)); err != nil {
			t.Fatalf("apply legacy migration %s: %v", file, err)
		}
	}
}

func assertCurrentSchema(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	passwordHash, err := columnExistsFromPool(ctx, pool, "users", "password_hash")
	if err != nil || !passwordHash {
		t.Fatalf("users.password_hash exists=%v error=%v", passwordHash, err)
	}
	for _, table := range []string{"businesses", "business_memberships", "audit_events", "refresh_sessions", "whatsapp_onboarding_sessions", "whatsapp_webhook_events"} {
		var exists bool
		if err := pool.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, table).Scan(&exists); err != nil {
			t.Fatalf("inspect table %s: %v", table, err)
		}
		if !exists {
			t.Fatalf("required table %s is missing", table)
		}
	}
	sourceEvent, err := columnExistsFromPool(ctx, pool, "whatsapp_messages", "source_webhook_event_id")
	if err != nil || !sourceEvent {
		t.Fatalf("whatsapp_messages.source_webhook_event_id exists=%v error=%v", sourceEvent, err)
	}
	var payloadType, sourceNullable string
	if err := pool.QueryRow(ctx, `
		SELECT
			(SELECT data_type FROM information_schema.columns
			 WHERE table_schema = current_schema()
			   AND table_name = 'whatsapp_webhook_events' AND column_name = 'payload'),
			(SELECT is_nullable FROM information_schema.columns
			 WHERE table_schema = current_schema()
			   AND table_name = 'whatsapp_messages' AND column_name = 'source_webhook_event_id')
	`).Scan(&payloadType, &sourceNullable); err != nil {
		t.Fatalf("inspect Task 6.2 column definitions: %v", err)
	}
	if payloadType != "jsonb" {
		t.Fatalf("whatsapp_webhook_events.payload type=%q, want jsonb", payloadType)
	}
	if sourceNullable != "YES" {
		t.Fatalf("whatsapp_messages.source_webhook_event_id nullable=%q, want YES", sourceNullable)
	}

	var foreignKeyValidated bool
	var deleteAction string
	var foreignKeyDefinition string
	if err := pool.QueryRow(ctx, `
		SELECT convalidated, confdeltype::text, pg_get_constraintdef(oid)
		FROM pg_constraint
		WHERE conrelid = 'whatsapp_messages'::regclass
		  AND conname = 'whatsapp_messages_source_webhook_event_id_fkey'
	`).Scan(&foreignKeyValidated, &deleteAction, &foreignKeyDefinition); err != nil {
		t.Fatalf("inspect source webhook event foreign key: %v", err)
	}
	if !foreignKeyValidated || deleteAction != "r" ||
		!strings.Contains(foreignKeyDefinition, "FOREIGN KEY (source_webhook_event_id) REFERENCES whatsapp_webhook_events(id) ON DELETE RESTRICT") {
		t.Fatalf(
			"source webhook event FK validated=%v delete_action=%q definition=%q",
			foreignKeyValidated, deleteAction, foreignKeyDefinition,
		)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin historical message compatibility check: %v", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var historicalSource *string
	if err := tx.QueryRow(ctx, `
		INSERT INTO whatsapp_messages (
			provider_message_id, sender_provider_id, business_phone_number_id,
			message_type, normalized_content, processing_status, received_at
		) VALUES ('migration-history-check', 'historical-sender', 'historical-phone',
			'text', '{}'::jsonb, 'RECEIVED', NOW())
		RETURNING source_webhook_event_id::text
	`).Scan(&historicalSource); err != nil {
		t.Fatalf("insert historical message without source event: %v", err)
	}
	if historicalSource != nil {
		t.Fatalf("historical message source event=%q, want NULL", *historicalSource)
	}
}

func columnExistsFromPool(ctx context.Context, pool *pgxpool.Pool, table, column string) (bool, error) {
	var exists bool
	err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = current_schema()
			  AND table_name = $1 AND column_name = $2
		)
	`, table, column).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}
