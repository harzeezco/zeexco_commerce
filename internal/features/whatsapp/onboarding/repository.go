package onboarding

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

const sessionColumns = `
id::text, provider, provider_user_id, business_phone_number_id, status,
expires_at, welcome_delivery_started_at, welcome_sent_at, completed_at,
completion_delivery_started_at, completion_sent_at, linked_user_id::text,
linked_business_id::text, created_at, updated_at`

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) PrepareWelcome(
	ctx context.Context,
	providerUserID string,
	businessPhoneNumberID string,
	tokenHash []byte,
	now time.Time,
	expiresAt time.Time,
	claimBefore time.Time,
) (_ WelcomePreparation, err error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return WelcomePreparation{}, fmt.Errorf("begin onboarding welcome: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, ProviderMetaWhatsApp+":"+providerUserID); err != nil {
		return WelcomePreparation{}, fmt.Errorf("lock onboarding identity: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE whatsapp_onboarding_sessions
		SET status = $3, updated_at = $2
		WHERE provider = $1 AND provider_user_id = $4
		  AND status = $5 AND expires_at <= $2
	`, ProviderMetaWhatsApp, now, StatusExpired, providerUserID, StatusRegistrationPending); err != nil {
		return WelcomePreparation{}, fmt.Errorf("expire onboarding session: %w", err)
	}

	var session Session
	err = scanSession(tx.QueryRow(ctx, `
		SELECT `+sessionColumns+`
		FROM whatsapp_onboarding_sessions
		WHERE provider = $1 AND provider_user_id = $2 AND status = $3
		FOR UPDATE
	`, ProviderMetaWhatsApp, providerUserID, StatusRegistrationPending), &session)
	if errors.Is(err, pgx.ErrNoRows) {
		err = scanSession(tx.QueryRow(ctx, `
			INSERT INTO whatsapp_onboarding_sessions (
				provider, provider_user_id, business_phone_number_id, token_hash,
				status, expires_at, welcome_delivery_started_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING `+sessionColumns,
			ProviderMetaWhatsApp, providerUserID, businessPhoneNumberID, tokenHash,
			StatusRegistrationPending, expiresAt, now,
		), &session)
		if err != nil {
			return WelcomePreparation{}, fmt.Errorf("create onboarding session: %w", err)
		}
		if err := insertOnboardingAudit(ctx, tx, "", "", session.ID, "WHATSAPP_ONBOARDING_STARTED"); err != nil {
			return WelcomePreparation{}, err
		}
	} else if err != nil {
		return WelcomePreparation{}, fmt.Errorf("load onboarding session: %w", err)
	} else {
		if session.WelcomeSentAt != nil ||
			(session.WelcomeDeliveryStartedAt != nil && session.WelcomeDeliveryStartedAt.After(claimBefore)) {
			if err := tx.Commit(ctx); err != nil {
				return WelcomePreparation{}, fmt.Errorf("commit onboarding welcome lookup: %w", err)
			}
			return WelcomePreparation{Session: session}, nil
		}
		err = scanSession(tx.QueryRow(ctx, `
			UPDATE whatsapp_onboarding_sessions
			SET token_hash = $2, expires_at = $3, business_phone_number_id = $4,
			    welcome_delivery_started_at = $5, updated_at = $5
			WHERE id = $1
			RETURNING `+sessionColumns,
			session.ID, tokenHash, expiresAt, businessPhoneNumberID, now,
		), &session)
		if err != nil {
			return WelcomePreparation{}, fmt.Errorf("claim onboarding welcome retry: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return WelcomePreparation{}, fmt.Errorf("commit onboarding welcome: %w", err)
	}
	return WelcomePreparation{Session: session, Send: true}, nil
}

func (r *Repository) MarkWelcomeSent(ctx context.Context, sessionID string, sentAt time.Time) error {
	_, err := r.db.Exec(ctx, `
		UPDATE whatsapp_onboarding_sessions
		SET welcome_sent_at = $2, welcome_delivery_started_at = NULL, updated_at = $2
		WHERE id = $1 AND welcome_sent_at IS NULL
	`, sessionID, sentAt)
	if err != nil {
		return fmt.Errorf("mark onboarding welcome sent: %w", err)
	}
	return nil
}

func (r *Repository) ReleaseWelcomeClaim(ctx context.Context, sessionID string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE whatsapp_onboarding_sessions
		SET welcome_delivery_started_at = NULL, updated_at = NOW()
		WHERE id = $1 AND welcome_sent_at IS NULL
	`, sessionID)
	return err
}

func (r *Repository) FindByToken(ctx context.Context, tokenHash []byte) (Session, error) {
	var session Session
	err := scanSession(r.db.QueryRow(ctx, `
		SELECT `+sessionColumns+` FROM whatsapp_onboarding_sessions WHERE token_hash = $1
	`, tokenHash), &session)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrInvalidSession
	}
	if err != nil {
		return Session{}, fmt.Errorf("find onboarding session: %w", err)
	}
	return session, nil
}

func (r *Repository) Complete(
	ctx context.Context,
	tokenHash []byte,
	input authmodels.RegistrationInput,
	now time.Time,
) (_ Completion, err error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Completion{}, fmt.Errorf("begin onboarding completion: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	var session Session
	err = scanSession(tx.QueryRow(ctx, `
		SELECT `+sessionColumns+`
		FROM whatsapp_onboarding_sessions WHERE token_hash = $1 FOR UPDATE
	`, tokenHash), &session)
	if errors.Is(err, pgx.ErrNoRows) {
		return Completion{}, ErrInvalidSession
	}
	if err != nil {
		return Completion{}, fmt.Errorf("lock onboarding session: %w", err)
	}
	if session.Status == StatusExpired {
		return Completion{}, ErrSessionExpired
	}
	if session.Status != StatusRegistrationPending {
		return Completion{}, ErrSessionConsumed
	}
	if !now.Before(session.ExpiresAt) {
		if _, err := tx.Exec(ctx, `UPDATE whatsapp_onboarding_sessions SET status = $2, updated_at = $3 WHERE id = $1`, session.ID, StatusExpired, now); err != nil {
			return Completion{}, fmt.Errorf("expire onboarding registration: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return Completion{}, fmt.Errorf("commit expired onboarding registration: %w", err)
		}
		return Completion{}, ErrSessionExpired
	}

	registration := authmodels.Registration{}
	err = tx.QueryRow(ctx, `
		INSERT INTO users (first_name, last_name, email, password_hash, status)
		VALUES ($1, $2, NULLIF($3, ''), NULL, $4)
		RETURNING id::text, first_name, last_name, COALESCE(email, ''), status, created_at, updated_at
	`, input.FirstName, input.LastName, input.Email, authmodels.UserStatusActive).Scan(
		&registration.User.ID, &registration.User.FirstName, &registration.User.LastName,
		&registration.User.Email, &registration.User.Status, &registration.User.CreatedAt,
		&registration.User.UpdatedAt,
	)
	if uniqueConstraint(err, "users_email_unique") {
		return Completion{}, authmodels.ErrEmailAlreadyRegistered
	}
	if err != nil {
		return Completion{}, fmt.Errorf("create onboarding user: %w", err)
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO businesses (name, status) VALUES ($1, $2)
		RETURNING id::text, name, status, created_at, updated_at
	`, input.BusinessName, businessmodels.StatusActive).Scan(
		&registration.Business.ID, &registration.Business.Name, &registration.Business.Status,
		&registration.Business.CreatedAt, &registration.Business.UpdatedAt,
	)
	if err != nil {
		return Completion{}, fmt.Errorf("create onboarding business: %w", err)
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO business_memberships (business_id, user_id, role)
		VALUES ($1, $2, $3)
		RETURNING id::text, business_id::text, user_id::text, role, created_at
	`, registration.Business.ID, registration.User.ID, businessmodels.RoleOwner).Scan(
		&registration.Membership.ID, &registration.Membership.BusinessID,
		&registration.Membership.UserID, &registration.Membership.Role,
		&registration.Membership.CreatedAt,
	)
	if err != nil {
		return Completion{}, fmt.Errorf("create onboarding owner membership: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO whatsapp_identities (user_id, provider, provider_user_id)
		VALUES ($1, $2, $3)
	`, registration.User.ID, session.Provider, session.ProviderUserID); uniqueConstraint(err, "whatsapp_identity_unique") {
		return Completion{}, ErrIdentityConflict
	} else if err != nil {
		return Completion{}, fmt.Errorf("link onboarding WhatsApp identity: %w", err)
	}
	if err := insertOnboardingAudit(ctx, tx, registration.User.ID, registration.Business.ID, session.ID, "USER_REGISTERED"); err != nil {
		return Completion{}, err
	}
	if err := insertOnboardingAudit(ctx, tx, registration.User.ID, registration.Business.ID, session.ID, "BUSINESS_CREATED"); err != nil {
		return Completion{}, err
	}
	if err := insertOnboardingAudit(ctx, tx, registration.User.ID, registration.Business.ID, session.ID, "WHATSAPP_IDENTITY_LINKED"); err != nil {
		return Completion{}, err
	}
	if err := insertOnboardingAudit(ctx, tx, registration.User.ID, registration.Business.ID, session.ID, "WHATSAPP_ONBOARDING_COMPLETED"); err != nil {
		return Completion{}, err
	}
	err = scanSession(tx.QueryRow(ctx, `
		UPDATE whatsapp_onboarding_sessions
		SET status = $2, completed_at = $3, linked_user_id = $4,
		    linked_business_id = $5, updated_at = $3
		WHERE id = $1
		RETURNING `+sessionColumns,
		session.ID, StatusFirstProductPending, now, registration.User.ID, registration.Business.ID,
	), &session)
	if err != nil {
		return Completion{}, fmt.Errorf("complete onboarding session: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Completion{}, fmt.Errorf("commit onboarding completion: %w", err)
	}
	return Completion{Session: session, Registration: registration}, nil
}

func (r *Repository) ClaimCompletionForUser(
	ctx context.Context,
	userID string,
	now time.Time,
	claimBefore time.Time,
) (Session, bool, error) {
	var session Session
	err := scanSession(r.db.QueryRow(ctx, `
		UPDATE whatsapp_onboarding_sessions
		SET completion_delivery_started_at = $2, updated_at = $2
		WHERE id = (
			SELECT id FROM whatsapp_onboarding_sessions
			WHERE linked_user_id = $1 AND status = $3 AND completion_sent_at IS NULL
			  AND (completion_delivery_started_at IS NULL OR completion_delivery_started_at <= $4)
			ORDER BY completed_at DESC LIMIT 1 FOR UPDATE SKIP LOCKED
		)
		RETURNING `+sessionColumns,
		userID, now, StatusFirstProductPending, claimBefore,
	), &session)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, false, nil
	}
	if err != nil {
		return Session{}, false, fmt.Errorf("claim onboarding completion delivery: %w", err)
	}
	return session, true, nil
}

func (r *Repository) MarkCompletionSent(ctx context.Context, sessionID string, sentAt time.Time) error {
	_, err := r.db.Exec(ctx, `
		UPDATE whatsapp_onboarding_sessions
		SET completion_sent_at = $2, completion_delivery_started_at = NULL, updated_at = $2
		WHERE id = $1 AND completion_sent_at IS NULL
	`, sessionID, sentAt)
	if err != nil {
		return fmt.Errorf("mark onboarding completion sent: %w", err)
	}
	return nil
}

func (r *Repository) ReleaseCompletionClaim(ctx context.Context, sessionID string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE whatsapp_onboarding_sessions
		SET completion_delivery_started_at = NULL, updated_at = NOW()
		WHERE id = $1 AND completion_sent_at IS NULL
	`, sessionID)
	return err
}

func scanSession(row pgx.Row, session *Session) error {
	return row.Scan(
		&session.ID, &session.Provider, &session.ProviderUserID,
		&session.BusinessPhoneNumberID, &session.Status, &session.ExpiresAt,
		&session.WelcomeDeliveryStartedAt, &session.WelcomeSentAt, &session.CompletedAt,
		&session.CompletionDeliveryStartedAt, &session.CompletionSentAt,
		&session.LinkedUserID, &session.LinkedBusinessID, &session.CreatedAt, &session.UpdatedAt,
	)
}

func insertOnboardingAudit(ctx context.Context, tx pgx.Tx, userID, businessID, sessionID, eventType string) error {
	var user any
	var business any
	if userID != "" {
		user = userID
	}
	if businessID != "" {
		business = businessID
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (actor_user_id, business_id, event_type, metadata)
		VALUES ($1, $2, $3, jsonb_build_object('onboarding_session_id', $4::text))
	`, user, business, eventType, sessionID); err != nil {
		return fmt.Errorf("create onboarding audit event: %w", err)
	}
	return nil
}

func uniqueConstraint(err error, constraint string) bool {
	var postgresError *pgconn.PgError
	return errors.As(err, &postgresError) && postgresError.Code == "23505" && postgresError.ConstraintName == constraint
}
