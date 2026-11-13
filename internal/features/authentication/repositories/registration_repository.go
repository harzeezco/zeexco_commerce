package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	authmodels "github.com/harzeezco/zeexco_commerce/internal/features/authentication/models"
	businessmodels "github.com/harzeezco/zeexco_commerce/internal/features/business/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	auditEventUserRegistered        = "USER_REGISTERED"
	auditEventBusinessCreated       = "BUSINESS_CREATED"
	auditEventVerificationRequested = "EMAIL_VERIFICATION_REQUESTED"
	defaultEmailVerificationTTL     = 24 * time.Hour
)

type RegistrationRepository struct {
	db              *pgxpool.Pool
	verificationTTL time.Duration
	now             func() time.Time
}

func NewRegistrationRepository(db *pgxpool.Pool) *RegistrationRepository {
	return NewRegistrationRepositoryWithVerificationTTL(db, defaultEmailVerificationTTL)
}

func NewRegistrationRepositoryWithVerificationTTL(db *pgxpool.Pool, ttl time.Duration) *RegistrationRepository {
	return &RegistrationRepository{db: db, verificationTTL: ttl, now: time.Now}
}

func (r *RegistrationRepository) Register(
	ctx context.Context,
	input authmodels.RegistrationInput,
) (_ *authmodels.Registration, err error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin registration transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	registration := &authmodels.Registration{}
	err = tx.QueryRow(ctx, `
		INSERT INTO users (
			first_name,
			last_name,
			email,
			password_hash,
			status
		) VALUES ($1, $2, $3, $4, $5)
		RETURNING id::text, first_name, last_name, email, status, created_at, updated_at
	`,
		input.FirstName,
		input.LastName,
		input.Email,
		input.PasswordHash,
		authmodels.UserStatusPending,
	).Scan(
		&registration.User.ID,
		&registration.User.FirstName,
		&registration.User.LastName,
		&registration.User.Email,
		&registration.User.Status,
		&registration.User.CreatedAt,
		&registration.User.UpdatedAt,
	)
	if isUniqueViolation(err) {
		return nil, authmodels.ErrEmailAlreadyRegistered
	}
	if err != nil {
		return nil, fmt.Errorf("create registration user: %w", err)
	}

	err = tx.QueryRow(ctx, `
		INSERT INTO businesses (name, status)
		VALUES ($1, $2)
		RETURNING id::text, name, status, created_at, updated_at
	`, input.BusinessName, businessmodels.StatusActive).Scan(
		&registration.Business.ID,
		&registration.Business.Name,
		&registration.Business.Status,
		&registration.Business.CreatedAt,
		&registration.Business.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create registration business: %w", err)
	}

	err = tx.QueryRow(ctx, `
		INSERT INTO business_memberships (business_id, user_id, role)
		VALUES ($1, $2, $3)
		RETURNING id::text, business_id::text, user_id::text, role, created_at
	`,
		registration.Business.ID,
		registration.User.ID,
		businessmodels.RoleOwner,
	).Scan(
		&registration.Membership.ID,
		&registration.Membership.BusinessID,
		&registration.Membership.UserID,
		&registration.Membership.Role,
		&registration.Membership.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create owner membership: %w", err)
	}

	if err := insertAuditEvent(ctx, tx, registration.User.ID, registration.Business.ID, auditEventUserRegistered); err != nil {
		return nil, err
	}
	if err := insertAuditEvent(ctx, tx, registration.User.ID, registration.Business.ID, auditEventBusinessCreated); err != nil {
		return nil, err
	}

	var verificationID string
	expiresAt := r.now().UTC().Add(r.verificationTTL)
	if err := tx.QueryRow(ctx, `
		INSERT INTO email_verifications (user_id, expires_at)
		VALUES ($1, $2)
		RETURNING id::text
	`, registration.User.ID, expiresAt).Scan(&verificationID); err != nil {
		return nil, fmt.Errorf("create email verification: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO email_delivery_jobs (verification_id, job_type)
		VALUES ($1, 'EMAIL_VERIFICATION')
	`, verificationID); err != nil {
		return nil, fmt.Errorf("create verification email job: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (actor_user_id, business_id, event_type, metadata)
		VALUES ($1, $2, $3, jsonb_build_object('verification_id', $4::text))
	`, registration.User.ID, registration.Business.ID, auditEventVerificationRequested, verificationID); err != nil {
		return nil, fmt.Errorf("create email verification audit event: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit registration transaction: %w", err)
	}
	return registration, nil
}

func insertAuditEvent(
	ctx context.Context,
	tx pgx.Tx,
	userID string,
	businessID string,
	eventType string,
) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (actor_user_id, business_id, event_type)
		VALUES ($1, $2, $3)
	`, userID, businessID, eventType); err != nil {
		return fmt.Errorf("create registration audit event: %w", err)
	}
	return nil
}

func isUniqueViolation(err error) bool {
	var postgresError *pgconn.PgError
	return errors.As(err, &postgresError) && postgresError.Code == "23505"
}
