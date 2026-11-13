package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	authmodels "github.com/harzeezco/zeexco_commerce/internal/features/authentication/models"
	businessmodels "github.com/harzeezco/zeexco_commerce/internal/features/business/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AuthenticationRepository struct {
	db *pgxpool.Pool
}

func NewAuthenticationRepository(db *pgxpool.Pool) *AuthenticationRepository {
	return &AuthenticationRepository{db: db}
}

func (r *AuthenticationRepository) FindCredentialUser(
	ctx context.Context,
	email string,
) (authmodels.CredentialUser, error) {
	var user authmodels.CredentialUser
	err := r.db.QueryRow(ctx, `
		SELECT id::text, first_name, last_name, email, status,
		       created_at, updated_at, COALESCE(password_hash, '')
		FROM users
		WHERE lower(email) = $1
	`, email).Scan(
		&user.ID, &user.FirstName, &user.LastName, &user.Email, &user.Status,
		&user.CreatedAt, &user.UpdatedAt, &user.PasswordHash,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return authmodels.CredentialUser{}, authmodels.ErrCredentialsNotFound
	}
	if err != nil {
		return authmodels.CredentialUser{}, fmt.Errorf("find credential user: %w", err)
	}
	return user, nil
}

func (r *AuthenticationRepository) CreateRefreshSession(
	ctx context.Context,
	input authmodels.RefreshSessionInput,
) (_ authmodels.RefreshSession, err error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return authmodels.RefreshSession{}, fmt.Errorf("begin refresh session transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	session, err := insertRefreshSession(ctx, tx, input)
	if err != nil {
		return authmodels.RefreshSession{}, err
	}
	if err := insertAuthenticationAuditEvent(ctx, tx, input.UserID, "USER_LOGGED_IN"); err != nil {
		return authmodels.RefreshSession{}, err
	}
	if err := insertAuthenticationAuditEvent(ctx, tx, input.UserID, "REFRESH_SESSION_CREATED"); err != nil {
		return authmodels.RefreshSession{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return authmodels.RefreshSession{}, fmt.Errorf("commit refresh session: %w", err)
	}
	return session, nil
}

func (r *AuthenticationRepository) RotateRefreshSession(
	ctx context.Context,
	input authmodels.RotationInput,
) (_ authmodels.RefreshSession, err error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return authmodels.RefreshSession{}, fmt.Errorf("begin refresh rotation: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	var current authmodels.RefreshSession
	err = tx.QueryRow(ctx, `
		SELECT id::text, user_id::text, token_hash, created_at, expires_at,
		       last_used_at, revoked_at, replaced_by_session_id::text
		FROM refresh_sessions
		WHERE token_hash = $1
		FOR UPDATE
	`, input.CurrentTokenHash).Scan(
		&current.ID, &current.UserID, &current.TokenHash, &current.CreatedAt,
		&current.ExpiresAt, &current.LastUsedAt, &current.RevokedAt,
		&current.ReplacedBySessionID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return authmodels.RefreshSession{}, authmodels.ErrRefreshTokenInvalid
	}
	if err != nil {
		return authmodels.RefreshSession{}, fmt.Errorf("lock refresh session: %w", err)
	}
	if current.RevokedAt != nil || !input.RotatedAt.Before(current.ExpiresAt) {
		return authmodels.RefreshSession{}, authmodels.ErrRefreshTokenInvalid
	}

	replacement, err := insertRefreshSession(ctx, tx, authmodels.RefreshSessionInput{
		UserID: current.UserID, TokenHash: input.NewTokenHash,
		CreatedAt: input.RotatedAt, ExpiresAt: input.NewExpiresAt,
	})
	if err != nil {
		return authmodels.RefreshSession{}, err
	}
	commandTag, err := tx.Exec(ctx, `
		UPDATE refresh_sessions
		SET last_used_at = $2, revoked_at = $2, replaced_by_session_id = $3
		WHERE id = $1 AND revoked_at IS NULL
	`, current.ID, input.RotatedAt, replacement.ID)
	if err != nil {
		return authmodels.RefreshSession{}, fmt.Errorf("consume refresh session: %w", err)
	}
	if commandTag.RowsAffected() != 1 {
		return authmodels.RefreshSession{}, authmodels.ErrRefreshTokenInvalid
	}
	if err := insertAuthenticationAuditEvent(ctx, tx, current.UserID, "REFRESH_SESSION_ROTATED"); err != nil {
		return authmodels.RefreshSession{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return authmodels.RefreshSession{}, fmt.Errorf("commit refresh rotation: %w", err)
	}
	return replacement, nil
}

func (r *AuthenticationRepository) RevokeRefreshSession(
	ctx context.Context,
	tokenHash []byte,
	revokedAt time.Time,
) (string, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin refresh revocation: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	var sessionID string
	var userID string
	err = tx.QueryRow(ctx, `
		SELECT id::text, user_id::text
		FROM refresh_sessions
		WHERE token_hash = $1 AND revoked_at IS NULL
		FOR UPDATE
	`, tokenHash).Scan(&sessionID, &userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", authmodels.ErrRefreshTokenInvalid
	}
	if err != nil {
		return "", fmt.Errorf("lock refresh session for logout: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE refresh_sessions SET revoked_at = $2, last_used_at = $2 WHERE id = $1
	`, sessionID, revokedAt); err != nil {
		return "", fmt.Errorf("revoke refresh session: %w", err)
	}
	if err := insertAuthenticationAuditEvent(ctx, tx, userID, "USER_LOGGED_OUT"); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit refresh revocation: %w", err)
	}
	return userID, nil
}

func (r *AuthenticationRepository) GetUserContext(
	ctx context.Context,
	userID string,
) (authmodels.UserContext, error) {
	var result authmodels.UserContext
	err := r.db.QueryRow(ctx, `
		SELECT id::text, first_name, last_name, COALESCE(email, ''), status, created_at, updated_at
		FROM users WHERE id = $1
	`, userID).Scan(
		&result.User.ID, &result.User.FirstName, &result.User.LastName,
		&result.User.Email, &result.User.Status, &result.User.CreatedAt, &result.User.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return authmodels.UserContext{}, authmodels.ErrCredentialsNotFound
	}
	if err != nil {
		return authmodels.UserContext{}, fmt.Errorf("load authenticated user: %w", err)
	}

	rows, err := r.db.Query(ctx, `
		SELECT b.id::text, b.name, b.status, b.created_at, b.updated_at,
		       bm.id::text, bm.business_id::text, bm.user_id::text, bm.role, bm.created_at
		FROM business_memberships bm
		JOIN businesses b ON b.id = bm.business_id
		WHERE bm.user_id = $1
		ORDER BY bm.created_at, bm.id
	`, userID)
	if err != nil {
		return authmodels.UserContext{}, fmt.Errorf("load user businesses: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var business authmodels.BusinessContext
		if err := rows.Scan(
			&business.Business.ID, &business.Business.Name, &business.Business.Status,
			&business.Business.CreatedAt, &business.Business.UpdatedAt,
			&business.Membership.ID, &business.Membership.BusinessID,
			&business.Membership.UserID, &business.Membership.Role,
			&business.Membership.CreatedAt,
		); err != nil {
			return authmodels.UserContext{}, fmt.Errorf("scan user business: %w", err)
		}
		result.Businesses = append(result.Businesses, business)
	}
	if err := rows.Err(); err != nil {
		return authmodels.UserContext{}, fmt.Errorf("iterate user businesses: %w", err)
	}
	return result, nil
}

func (r *AuthenticationRepository) FindBusinessMembership(
	ctx context.Context,
	userID string,
	businessID string,
) (businessmodels.Membership, error) {
	var membership businessmodels.Membership
	err := r.db.QueryRow(ctx, `
		SELECT id::text, business_id::text, user_id::text, role, created_at
		FROM business_memberships
		WHERE user_id = $1 AND business_id = $2
	`, userID, businessID).Scan(
		&membership.ID, &membership.BusinessID, &membership.UserID,
		&membership.Role, &membership.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return businessmodels.Membership{}, authmodels.ErrMembershipNotFound
	}
	if err != nil {
		return businessmodels.Membership{}, fmt.Errorf("find business membership: %w", err)
	}
	return membership, nil
}

func insertRefreshSession(
	ctx context.Context,
	tx pgx.Tx,
	input authmodels.RefreshSessionInput,
) (authmodels.RefreshSession, error) {
	var session authmodels.RefreshSession
	err := tx.QueryRow(ctx, `
		INSERT INTO refresh_sessions (user_id, token_hash, created_at, expires_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id::text, user_id::text, token_hash, created_at, expires_at,
		          last_used_at, revoked_at, replaced_by_session_id::text
	`, input.UserID, input.TokenHash, input.CreatedAt, input.ExpiresAt).Scan(
		&session.ID, &session.UserID, &session.TokenHash, &session.CreatedAt,
		&session.ExpiresAt, &session.LastUsedAt, &session.RevokedAt,
		&session.ReplacedBySessionID,
	)
	if err != nil {
		return authmodels.RefreshSession{}, fmt.Errorf("create refresh session: %w", err)
	}
	return session, nil
}

func insertAuthenticationAuditEvent(
	ctx context.Context,
	tx pgx.Tx,
	userID string,
	eventType string,
) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (actor_user_id, event_type)
		VALUES ($1, $2)
	`, userID, eventType); err != nil {
		return fmt.Errorf("create authentication audit event: %w", err)
	}
	return nil
}
