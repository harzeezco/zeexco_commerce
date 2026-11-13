package emailverification

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrVerificationExpired  = errors.New("email verification has expired")
	ErrVerificationConsumed = errors.New("email verification has already been consumed")
)

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) ClaimNext(
	ctx context.Context,
	now time.Time,
	staleBefore time.Time,
	renewalExpiry time.Time,
) (_ DeliveryJob, claimed bool, err error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return DeliveryJob{}, false, fmt.Errorf("begin email job claim: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	var job DeliveryJob
	err = tx.QueryRow(ctx, `
		SELECT j.id::text, j.verification_id::text, v.user_id::text,
		       u.email, u.first_name, v.expires_at, j.attempt_count
		FROM email_delivery_jobs j
		JOIN email_verifications v ON v.id = j.verification_id
		JOIN users u ON u.id = v.user_id
		WHERE v.verified_at IS NULL
		  AND (
		    (j.status IN ('PENDING', 'FAILED') AND j.available_at <= $1)
		    OR (j.status = 'PROCESSING' AND j.processing_started_at <= $2)
		  )
		ORDER BY j.available_at, j.created_at, j.id
		FOR UPDATE OF j SKIP LOCKED
		LIMIT 1
	`, now, staleBefore).Scan(
		&job.ID, &job.VerificationID, &job.UserID, &job.Email,
		&job.FirstName, &job.ExpiresAt, &job.AttemptCount,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return DeliveryJob{}, false, nil
	}
	if err != nil {
		return DeliveryJob{}, false, fmt.Errorf("select email job: %w", err)
	}
	if !now.Before(job.ExpiresAt) {
		if _, err := tx.Exec(ctx, `
			UPDATE email_verifications SET expires_at = $2 WHERE id = $1 AND verified_at IS NULL
		`, job.VerificationID, renewalExpiry); err != nil {
			return DeliveryJob{}, false, fmt.Errorf("renew unsent email verification: %w", err)
		}
		job.ExpiresAt = renewalExpiry
	}
	if _, err := tx.Exec(ctx, `
		UPDATE email_delivery_jobs
		SET status = 'PROCESSING', processing_started_at = $2,
		    attempt_count = attempt_count + 1, last_error = NULL, updated_at = $2
		WHERE id = $1
	`, job.ID, now); err != nil {
		return DeliveryJob{}, false, fmt.Errorf("claim email job: %w", err)
	}
	job.AttemptCount++
	if err := tx.Commit(ctx); err != nil {
		return DeliveryJob{}, false, fmt.Errorf("commit email job claim: %w", err)
	}
	return job, true, nil
}

func (r *Repository) MarkSent(ctx context.Context, jobID, providerMessageID string, sentAt time.Time) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE email_delivery_jobs
		SET status = 'SENT', processing_started_at = NULL, sent_at = $2,
		    provider_message_id = $3, last_error = NULL, updated_at = $2
		WHERE id = $1 AND status = 'PROCESSING'
	`, jobID, sentAt, providerMessageID)
	if err != nil {
		return fmt.Errorf("mark verification email sent: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return errors.New("verification email job is not processing")
	}
	return nil
}

func (r *Repository) MarkFailed(ctx context.Context, jobID string, failedAt, retryAt time.Time, message string) error {
	message = strings.Join(strings.Fields(message), " ")
	if len(message) > 512 {
		message = message[:512]
	}
	tag, err := r.db.Exec(ctx, `
		UPDATE email_delivery_jobs
		SET status = 'FAILED', processing_started_at = NULL, available_at = $3,
		    last_error = $4, updated_at = $2
		WHERE id = $1 AND status = 'PROCESSING'
	`, jobID, failedAt, retryAt, message)
	if err != nil {
		return fmt.Errorf("mark verification email failed: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return errors.New("verification email job is not processing")
	}
	return nil
}

func (r *Repository) Verify(ctx context.Context, verificationID string, now time.Time) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin email verification: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var userID string
	var expiresAt time.Time
	var verifiedAt *time.Time
	err = tx.QueryRow(ctx, `
		SELECT user_id::text, expires_at, verified_at
		FROM email_verifications WHERE id = $1 FOR UPDATE
	`, verificationID).Scan(&userID, &expiresAt, &verifiedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInvalidToken
	}
	if err != nil {
		return fmt.Errorf("lock email verification: %w", err)
	}
	if verifiedAt != nil {
		return ErrVerificationConsumed
	}
	if !now.Before(expiresAt) {
		return ErrVerificationExpired
	}
	tag, err := tx.Exec(ctx, `
		UPDATE users SET status = 'ACTIVE', updated_at = $2
		WHERE id = $1 AND status = 'PENDING'
	`, userID, now)
	if err != nil {
		return fmt.Errorf("activate verified user: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrVerificationConsumed
	}
	if _, err := tx.Exec(ctx, `UPDATE email_verifications SET verified_at = $2 WHERE id = $1`, verificationID, now); err != nil {
		return fmt.Errorf("consume email verification: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (actor_user_id, event_type, metadata)
		VALUES ($1, 'EMAIL_VERIFIED', jsonb_build_object('verification_id', $2::text))
	`, userID, verificationID); err != nil {
		return fmt.Errorf("audit email verification: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit email verification: %w", err)
	}
	return nil
}
