package database

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"

	"github.com/golang-migrate/migrate/v4"
	migratepgx "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	migrationassets "github.com/harzeezco/zeexco_commerce/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
)

var migrationFilename = regexp.MustCompile(`^(\d+)_([a-z0-9_]+)\.up\.sql$`)

type MigrationReport struct {
	Applied                          []string
	Baselined                        []string
	LegacyActiveUsersWithoutPassword int64
}

type SchemaStatus struct {
	MigrationTracking                 bool
	MigrationTrackingColumns          string
	MigrationTrackingRows             int64
	TrackedVersion                    int64
	TrackingDirty                     bool
	Users                             bool
	WhatsAppIdentities                bool
	PasswordHashColumn                bool
	Businesses                        bool
	BusinessMemberships               bool
	AuditEvents                       bool
	RefreshSessions                   bool
	WhatsAppMessages                  bool
	WhatsAppResolutionColumn          bool
	WhatsAppOnboardingSessions        bool
	EmailVerifications                bool
	EmailDeliveryJobs                 bool
	CaseInsensitiveDuplicateEmailSets int64
	ActiveUsersWithoutPassword        int64
}

type migrationFile struct {
	version int
	name    string
	file    string
}

// Migrate applies pending embedded SQL migrations using the project's existing
// golang-migrate schema_migrations(version, dirty) tracking format. The driver
// provides a PostgreSQL advisory lock, so concurrent instances serialize.
func Migrate(ctx context.Context, pool *pgxpool.Pool) (MigrationReport, error) {
	if pool == nil {
		return MigrationReport{}, errors.New("database pool is required")
	}
	files, err := loadMigrations()
	if err != nil {
		return MigrationReport{}, err
	}
	status, err := InspectSchema(ctx, pool)
	if err != nil {
		return MigrationReport{}, err
	}
	if status.MigrationTracking && status.MigrationTrackingColumns != "version,dirty" {
		return MigrationReport{}, fmt.Errorf(
			"schema_migrations has incompatible columns %q; expected version,dirty",
			status.MigrationTrackingColumns,
		)
	}

	baselineVersion := 0
	report := MigrationReport{}
	if !status.MigrationTracking && hasManagedSchema(status) {
		baselineVersion, report.Baselined, err = inspectLegacyBaseline(ctx, pool, files, status)
		if err != nil {
			return MigrationReport{}, err
		}
	}

	sourceDriver, err := iofs.New(migrationassets.Files, ".")
	if err != nil {
		return MigrationReport{}, fmt.Errorf("open embedded migrations: %w", err)
	}
	sqlDB := stdlib.OpenDB(*pool.Config().ConnConfig.Copy())
	databaseDriver, err := migratepgx.WithInstance(sqlDB, &migratepgx.Config{
		DatabaseName: pool.Config().ConnConfig.Database,
	})
	if err != nil {
		_ = sourceDriver.Close()
		_ = sqlDB.Close()
		return MigrationReport{}, fmt.Errorf("initialize migration database driver: %w", err)
	}
	runner, err := migrate.NewWithInstance("iofs", sourceDriver, "pgx5", databaseDriver)
	if err != nil {
		_ = sourceDriver.Close()
		_ = databaseDriver.Close()
		return MigrationReport{}, fmt.Errorf("initialize migration runner: %w", err)
	}
	defer func() { _, _ = runner.Close() }()

	if baselineVersion > 0 {
		if err := runner.Force(baselineVersion); err != nil {
			return MigrationReport{}, fmt.Errorf("record verified legacy migration baseline %06d: %w", baselineVersion, err)
		}
	}
	beforeVersion, dirty, err := migrationVersion(runner)
	if err != nil {
		return MigrationReport{}, err
	}
	if dirty {
		return MigrationReport{}, fmt.Errorf("database migration version %06d is dirty; inspect and repair the failed migration before retrying", beforeVersion)
	}
	initialVersion := beforeVersion
	if beforeVersion < 5 {
		if err := runner.Migrate(5); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			return MigrationReport{}, fmt.Errorf("apply prerequisite database migrations through 000005: %w", err)
		}
		beforeVersion, dirty, err = migrationVersion(runner)
		if err != nil {
			return MigrationReport{}, err
		}
		if dirty || beforeVersion != 5 {
			return MigrationReport{}, fmt.Errorf("database did not reach clean prerequisite migration 000005")
		}
	}
	if beforeVersion < 6 {
		if err := preflightBusinessFoundation(ctx, pool); err != nil {
			return MigrationReport{}, err
		}
	}

	if err := runner.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return MigrationReport{}, fmt.Errorf("apply pending database migrations: %w", err)
	}
	afterVersion, dirty, err := migrationVersion(runner)
	if err != nil {
		return MigrationReport{}, err
	}
	if dirty {
		return MigrationReport{}, fmt.Errorf("database migration version %06d remained dirty", afterVersion)
	}
	for _, file := range files {
		if file.version > initialVersion && file.version <= afterVersion {
			report.Applied = append(report.Applied, migrationLabel(file))
		}
	}

	finalStatus, err := InspectSchema(ctx, pool)
	if err != nil {
		return MigrationReport{}, err
	}
	report.LegacyActiveUsersWithoutPassword = finalStatus.ActiveUsersWithoutPassword
	if err := requireCurrentSchema(finalStatus); err != nil {
		return MigrationReport{}, err
	}
	return report, nil
}

// InspectSchema returns migration-relevant state without changing the database
// or exposing email values or other user data.
func InspectSchema(ctx context.Context, pool *pgxpool.Pool) (SchemaStatus, error) {
	if pool == nil {
		return SchemaStatus{}, errors.New("database pool is required")
	}
	connection, err := pool.Acquire(ctx)
	if err != nil {
		return SchemaStatus{}, fmt.Errorf("acquire schema inspection connection: %w", err)
	}
	defer connection.Release()

	var status SchemaStatus
	checks := []struct {
		name   string
		target *bool
	}{
		{"schema_migrations", &status.MigrationTracking},
		{"users", &status.Users},
		{"whatsapp_identities", &status.WhatsAppIdentities},
		{"businesses", &status.Businesses},
		{"business_memberships", &status.BusinessMemberships},
		{"audit_events", &status.AuditEvents},
		{"refresh_sessions", &status.RefreshSessions},
		{"whatsapp_messages", &status.WhatsAppMessages},
		{"whatsapp_onboarding_sessions", &status.WhatsAppOnboardingSessions},
		{"email_verifications", &status.EmailVerifications},
		{"email_delivery_jobs", &status.EmailDeliveryJobs},
	}
	for _, check := range checks {
		*check.target, err = relationExists(ctx, connection, check.name)
		if err != nil {
			return SchemaStatus{}, err
		}
	}
	if status.MigrationTracking {
		if err := connection.QueryRow(ctx, `
			SELECT COALESCE(string_agg(column_name, ',' ORDER BY ordinal_position), '')
			FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = 'schema_migrations'
		`).Scan(&status.MigrationTrackingColumns); err != nil {
			return SchemaStatus{}, fmt.Errorf("inspect migration tracking columns: %w", err)
		}
		if err := connection.QueryRow(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&status.MigrationTrackingRows); err != nil {
			return SchemaStatus{}, fmt.Errorf("count migration tracking rows: %w", err)
		}
		if status.MigrationTrackingColumns == "version,dirty" && status.MigrationTrackingRows == 1 {
			if err := connection.QueryRow(ctx, `SELECT version, dirty FROM schema_migrations LIMIT 1`).Scan(
				&status.TrackedVersion, &status.TrackingDirty,
			); err != nil {
				return SchemaStatus{}, fmt.Errorf("read migration version: %w", err)
			}
		}
	}
	if !status.Users {
		return status, nil
	}
	status.PasswordHashColumn, err = columnExists(ctx, connection, "users", "password_hash")
	if err != nil {
		return SchemaStatus{}, err
	}
	if status.WhatsAppMessages {
		status.WhatsAppResolutionColumn, err = columnExists(ctx, connection, "whatsapp_messages", "resolved_user_id")
		if err != nil {
			return SchemaStatus{}, err
		}
	}
	if err := connection.QueryRow(ctx, `
		SELECT count(*) FROM (
			SELECT lower(email) FROM users WHERE email IS NOT NULL
			GROUP BY lower(email) HAVING count(*) > 1
		) duplicates
	`).Scan(&status.CaseInsensitiveDuplicateEmailSets); err != nil {
		return SchemaStatus{}, fmt.Errorf("count case-insensitive duplicate email groups: %w", err)
	}
	if status.PasswordHashColumn {
		if status.WhatsAppIdentities {
			err = connection.QueryRow(ctx, `
				SELECT count(*) FROM users u
				WHERE u.status = 'ACTIVE' AND u.password_hash IS NULL
				  AND NOT EXISTS (SELECT 1 FROM whatsapp_identities wi WHERE wi.user_id = u.id)
			`).Scan(&status.ActiveUsersWithoutPassword)
		} else {
			err = connection.QueryRow(ctx, `SELECT count(*) FROM users WHERE status = 'ACTIVE' AND password_hash IS NULL`).Scan(&status.ActiveUsersWithoutPassword)
		}
	} else {
		err = connection.QueryRow(ctx, `SELECT count(*) FROM users WHERE status = 'ACTIVE'`).Scan(&status.ActiveUsersWithoutPassword)
	}
	if err != nil {
		return SchemaStatus{}, fmt.Errorf("count active users without passwords: %w", err)
	}
	return status, nil
}

func migrationVersion(runner *migrate.Migrate) (int, bool, error) {
	version, dirty, err := runner.Version()
	if errors.Is(err, migrate.ErrNilVersion) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("read database migration version: %w", err)
	}
	return int(version), dirty, nil
}

func loadMigrations() ([]migrationFile, error) {
	files, err := fs.Glob(migrationassets.Files, "*.up.sql")
	if err != nil {
		return nil, fmt.Errorf("list embedded migrations: %w", err)
	}
	result := make([]migrationFile, 0, len(files))
	versions := make(map[int]string, len(files))
	for _, file := range files {
		matches := migrationFilename.FindStringSubmatch(filepath.Base(file))
		if matches == nil {
			return nil, fmt.Errorf("invalid migration filename %q", file)
		}
		version, err := strconv.Atoi(matches[1])
		if err != nil || version <= 0 {
			return nil, fmt.Errorf("invalid migration version in %q", file)
		}
		if existing, duplicate := versions[version]; duplicate {
			return nil, fmt.Errorf("duplicate migration version %06d in %q and %q", version, existing, file)
		}
		versions[version] = file
		result = append(result, migrationFile{version: version, name: matches[2], file: file})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].version < result[j].version })
	return result, nil
}

func inspectLegacyBaseline(
	ctx context.Context,
	pool *pgxpool.Pool,
	files []migrationFile,
	status SchemaStatus,
) (int, []string, error) {
	connection, err := pool.Acquire(ctx)
	if err != nil {
		return 0, nil, fmt.Errorf("acquire legacy schema inspection connection: %w", err)
	}
	defer connection.Release()

	users, err := relationExists(ctx, connection, "users")
	if err != nil || !users {
		return 0, nil, errors.New("managed legacy tables exist but required users table is missing")
	}
	if err := requireColumns(ctx, connection, "users", "id", "first_name", "last_name", "email", "status", "created_at", "updated_at"); err != nil {
		return 0, nil, fmt.Errorf("cannot baseline migration 000001: %w", err)
	}
	if err := requireConstraints(ctx, connection, "users", "users_status_check", "users_active_profile_check", "users_email_not_empty"); err != nil {
		return 0, nil, fmt.Errorf("cannot baseline migration 000001: %w", err)
	}
	baseline := 1
	if identities, err := relationExists(ctx, connection, "whatsapp_identities"); err != nil {
		return 0, nil, err
	} else if identities {
		if err := requireColumns(ctx, connection, "whatsapp_identities", "id", "user_id", "provider", "provider_user_id", "created_at"); err != nil {
			return 0, nil, fmt.Errorf("cannot baseline migration 000003: %w", err)
		}
		baseline = 3
	}
	if status.WhatsAppMessages {
		if baseline < 3 {
			return 0, nil, errors.New("legacy whatsapp_messages exists but whatsapp_identities is missing")
		}
		if err := requireColumns(ctx, connection, "whatsapp_messages", "id", "provider_message_id", "sender_provider_id", "business_phone_number_id", "message_type", "normalized_content", "processing_status", "provider_timestamp", "received_at", "created_at"); err != nil {
			return 0, nil, fmt.Errorf("cannot baseline migration 000004: %w", err)
		}
		baseline = 4
		if status.WhatsAppResolutionColumn {
			if err := requireConstraints(ctx, connection, "whatsapp_messages", "whatsapp_messages_identity_resolution_check"); err != nil {
				return 0, nil, fmt.Errorf("cannot baseline migration 000005: %w", err)
			}
			baseline = 5
		}
	}
	versionSixParts := []bool{status.PasswordHashColumn, status.Businesses, status.BusinessMemberships, status.AuditEvents}
	if anyTrue(versionSixParts) && !allTrue(versionSixParts) {
		return 0, nil, errors.New("legacy migration 000006 is partially applied; repair it explicitly before retrying")
	}
	if allTrue(versionSixParts) {
		if err := requireConstraints(ctx, connection, "users", "users_password_hash_not_empty", "users_active_password_hash_check"); err != nil {
			return 0, nil, fmt.Errorf("cannot baseline migration 000006: %w", err)
		}
		baseline = 6
	}
	if status.RefreshSessions {
		if baseline < 6 {
			return 0, nil, errors.New("legacy refresh_sessions exists before migration 000006 is complete")
		}
		if err := requireColumns(ctx, connection, "refresh_sessions", "id", "user_id", "token_hash", "created_at", "expires_at", "last_used_at", "revoked_at", "replaced_by_session_id"); err != nil {
			return 0, nil, fmt.Errorf("cannot baseline migration 000007: %w", err)
		}
		baseline = 7
	}
	if status.WhatsAppOnboardingSessions {
		if baseline < 7 {
			return 0, nil, errors.New("legacy whatsapp_onboarding_sessions exists before migration 000007 is complete")
		}
		if err := requireColumns(ctx, connection, "whatsapp_onboarding_sessions", "id", "provider", "provider_user_id", "business_phone_number_id", "token_hash", "status", "expires_at", "completed_at", "linked_user_id", "linked_business_id"); err != nil {
			return 0, nil, fmt.Errorf("cannot baseline migration 000008: %w", err)
		}
		baseline = 8
	}

	labels := make([]string, 0)
	found := false
	for _, file := range files {
		if file.version <= baseline {
			labels = append(labels, migrationLabel(file))
		}
		if file.version == baseline {
			found = true
		}
	}
	if !found {
		return 0, nil, fmt.Errorf("legacy schema baseline %06d has no matching migration file", baseline)
	}
	return baseline, labels, nil
}

func preflightBusinessFoundation(ctx context.Context, pool *pgxpool.Pool) error {
	var duplicateGroups int64
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM (
			SELECT lower(email) FROM users WHERE email IS NOT NULL
			GROUP BY lower(email) HAVING count(*) > 1
		) duplicates
	`).Scan(&duplicateGroups); err != nil {
		return fmt.Errorf("check case-insensitive email duplicates before migration 000006: %w", err)
	}
	if duplicateGroups > 0 {
		return fmt.Errorf("migration 000006 blocked by %d case-insensitive duplicate email group(s); resolve them explicitly without deleting data", duplicateGroups)
	}
	return nil
}

func requireCurrentSchema(status SchemaStatus) error {
	if !status.PasswordHashColumn || !status.Businesses || !status.BusinessMemberships || !status.AuditEvents || !status.RefreshSessions || !status.WhatsAppOnboardingSessions || !status.EmailVerifications || !status.EmailDeliveryJobs {
		return errors.New("migration runner completed but required current schema objects are missing")
	}
	return nil
}

func hasManagedSchema(status SchemaStatus) bool {
	return status.Users || status.WhatsAppIdentities || status.WhatsAppMessages || status.PasswordHashColumn || status.Businesses ||
		status.BusinessMemberships || status.AuditEvents || status.RefreshSessions || status.WhatsAppOnboardingSessions ||
		status.EmailVerifications || status.EmailDeliveryJobs
}

func relationExists(ctx context.Context, connection *pgxpool.Conn, relation string) (bool, error) {
	var exists bool
	if err := connection.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, relation).Scan(&exists); err != nil {
		return false, fmt.Errorf("inspect relation %q: %w", relation, err)
	}
	return exists, nil
}

func columnExists(ctx context.Context, connection *pgxpool.Conn, table, column string) (bool, error) {
	var exists bool
	if err := connection.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = $1 AND column_name = $2
		)
	`, table, column).Scan(&exists); err != nil {
		return false, fmt.Errorf("inspect column %s.%s: %w", table, column, err)
	}
	return exists, nil
}

func requireColumns(ctx context.Context, connection *pgxpool.Conn, table string, columns ...string) error {
	for _, column := range columns {
		exists, err := columnExists(ctx, connection, table, column)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("required column %s.%s is missing", table, column)
		}
	}
	return nil
}

func requireConstraints(ctx context.Context, connection *pgxpool.Conn, table string, constraints ...string) error {
	for _, constraint := range constraints {
		var exists bool
		if err := connection.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM pg_constraint
				WHERE conrelid = to_regclass($1) AND conname = $2
			)
		`, table, constraint).Scan(&exists); err != nil {
			return fmt.Errorf("inspect constraint %s.%s: %w", table, constraint, err)
		}
		if !exists {
			return fmt.Errorf("required constraint %s.%s is missing", table, constraint)
		}
	}
	return nil
}

func anyTrue(values []bool) bool {
	for _, value := range values {
		if value {
			return true
		}
	}
	return false
}

func allTrue(values []bool) bool {
	for _, value := range values {
		if !value {
			return false
		}
	}
	return true
}

func migrationLabel(file migrationFile) string {
	return fmt.Sprintf("%06d_%s", file.version, file.name)
}
