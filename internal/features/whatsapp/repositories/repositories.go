package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/harzeezco/zeexco_commerce/internal/features/identity"
	"github.com/harzeezco/zeexco_commerce/internal/features/whatsapp/ingestion"
	"github.com/harzeezco/zeexco_commerce/internal/features/whatsapp/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const metaWhatsAppProvider = "META_WHATSAPP"

const resolveWhatsAppIdentitySQL = `
SELECT user_id::text
FROM whatsapp_identities
WHERE provider = $1
  AND provider_user_id = $2
`

const storeWhatsAppMessageSQL = `
INSERT INTO whatsapp_messages (
    provider_message_id,
    sender_provider_id,
    business_phone_number_id,
    message_type,
    normalized_content,
    processing_status,
    provider_timestamp,
    received_at,
    source_webhook_event_id
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (provider_message_id) DO NOTHING
RETURNING id::text
`

type QueryRower interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type processingTransaction interface {
	QueryRower
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

type transactionStarter func(context.Context) (processingTransaction, error)

type WhatsAppIdentityRepository struct {
	db QueryRower
}

func NewWhatsAppIdentityRepository(db QueryRower) *WhatsAppIdentityRepository {
	return &WhatsAppIdentityRepository{db: db}
}

func (r *WhatsAppIdentityRepository) Resolve(
	ctx context.Context,
	providerUserID string,
) (identity.Resolution, error) {
	return r.resolveWith(r.db, ctx, providerUserID)
}

func (r *WhatsAppIdentityRepository) ResolveWithScope(
	ctx context.Context,
	scope any,
	providerUserID string,
) (identity.Resolution, error) {
	queryer, ok := scope.(QueryRower)
	if !ok || queryer == nil {
		return identity.Resolution{}, errors.New("invalid WhatsApp identity resolution scope")
	}

	return r.resolveWith(queryer, ctx, providerUserID)
}

func (r *WhatsAppIdentityRepository) resolveWith(
	queryer QueryRower,
	ctx context.Context,
	providerUserID string,
) (identity.Resolution, error) {
	var userID string
	err := queryer.QueryRow(
		ctx,
		resolveWhatsAppIdentitySQL,
		metaWhatsAppProvider,
		providerUserID,
	).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return identity.Resolution{Registered: false}, nil
	}
	if err != nil {
		return identity.Resolution{}, fmt.Errorf("resolve WhatsApp identity: %w", err)
	}

	return identity.Resolution{
		Registered: true,
		UserID:     userID,
	}, nil
}

type WhatsAppMessageRepository struct {
	db    QueryRower
	begin transactionStarter
}

func NewWhatsAppMessageRepository(db *pgxpool.Pool) *WhatsAppMessageRepository {
	return newWhatsAppMessageRepository(db, func(ctx context.Context) (processingTransaction, error) {
		return db.Begin(ctx)
	})
}

func newWhatsAppMessageRepository(
	db QueryRower,
	begin transactionStarter,
) *WhatsAppMessageRepository {
	return &WhatsAppMessageRepository{db: db, begin: begin}
}

func (r *WhatsAppMessageRepository) Store(
	ctx context.Context,
	message ingestion.Message,
) (bool, error) {
	content, err := json.Marshal(message.Content)
	if err != nil {
		return false, fmt.Errorf("encode normalized WhatsApp message content: %w", err)
	}

	var storedID string
	err = r.db.QueryRow(
		ctx,
		storeWhatsAppMessageSQL,
		message.ProviderMessageID,
		message.SenderProviderID,
		message.BusinessPhoneNumberID,
		message.MessageType,
		content,
		message.ProcessingStatus,
		message.ProviderTimestamp,
		message.ReceivedAt,
		nullableString(message.SourceWebhookEventID),
	).Scan(&storedID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("store WhatsApp message: %w", err)
	}

	return true, nil
}

type WhatsAppWebhookEventRepository struct {
	db *pgxpool.Pool
}

func NewWhatsAppWebhookEventRepository(db *pgxpool.Pool) *WhatsAppWebhookEventRepository {
	return &WhatsAppWebhookEventRepository{db: db}
}

func (r *WhatsAppWebhookEventRepository) Store(ctx context.Context, event models.WebhookEvent) (models.WebhookEvent, error) {
	if !json.Valid(event.Payload) {
		return models.WebhookEvent{}, errors.New("WhatsApp webhook payload must be valid JSON")
	}
	err := r.db.QueryRow(ctx, `
		INSERT INTO whatsapp_webhook_events (
			provider, provider_account_id, phone_number_id, event_type, payload,
			processing_status, received_at
		) VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7)
		RETURNING id::text
	`, event.Provider, nullableString(event.ProviderAccountID), nullableString(event.PhoneNumberID),
		event.EventType, []byte(event.Payload), event.ProcessingStatus, event.ReceivedAt,
	).Scan(&event.ID)
	if err != nil {
		return models.WebhookEvent{}, fmt.Errorf("insert WhatsApp webhook event: %w", err)
	}
	return event, nil
}

func (r *WhatsAppWebhookEventRepository) ClaimNext(
	ctx context.Context,
	now time.Time,
	retryBefore time.Time,
	staleBefore time.Time,
	maxAttempts int,
) (_ models.WebhookEvent, claimed bool, err error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return models.WebhookEvent{}, false, fmt.Errorf("begin WhatsApp webhook claim: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	var event models.WebhookEvent
	err = tx.QueryRow(ctx, `
		SELECT id::text, provider, COALESCE(provider_account_id, ''),
		       COALESCE(phone_number_id, ''), event_type, payload,
		       processing_status, received_at, processing_started_at,
		       processed_at, attempt_count, COALESCE(last_error_code, ''),
		       COALESCE(last_error_message, '')
		FROM whatsapp_webhook_events
		WHERE attempt_count < $1
		  AND (
		    processing_status = 'RECEIVED'
		    OR (processing_status = 'FAILED' AND updated_at <= $2)
		    OR (processing_status = 'PROCESSING' AND processing_started_at <= $3)
		  )
		ORDER BY received_at, id
		FOR UPDATE SKIP LOCKED
		LIMIT 1
	`, maxAttempts, retryBefore, staleBefore).Scan(
		&event.ID, &event.Provider, &event.ProviderAccountID, &event.PhoneNumberID,
		&event.EventType, &event.Payload, &event.ProcessingStatus, &event.ReceivedAt,
		&event.ProcessingStartedAt, &event.ProcessedAt, &event.AttemptCount,
		&event.LastErrorCode, &event.LastErrorMessage,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.WebhookEvent{}, false, nil
	}
	if err != nil {
		return models.WebhookEvent{}, false, fmt.Errorf("select WhatsApp webhook claim: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE whatsapp_webhook_events
		SET processing_status = 'PROCESSING', processing_started_at = $2,
		    processed_at = NULL, attempt_count = attempt_count + 1,
		    last_error_code = NULL, last_error_message = NULL, updated_at = $2
		WHERE id = $1
	`, event.ID, now); err != nil {
		return models.WebhookEvent{}, false, fmt.Errorf("claim WhatsApp webhook event: %w", err)
	}
	event.ProcessingStatus = ingestion.WebhookStatusProcessing
	event.ProcessingStartedAt = &now
	event.ProcessedAt = nil
	event.AttemptCount++
	if err := tx.Commit(ctx); err != nil {
		return models.WebhookEvent{}, false, fmt.Errorf("commit WhatsApp webhook claim: %w", err)
	}
	return event, true, nil
}

func (r *WhatsAppWebhookEventRepository) MarkProcessing(ctx context.Context, id string, at time.Time) error {
	return r.transition(ctx, id, `
		UPDATE whatsapp_webhook_events
		SET processing_status = 'PROCESSING', processing_started_at = $2,
			processed_at = NULL,
			attempt_count = attempt_count + 1, last_error_code = NULL,
			last_error_message = NULL, updated_at = $2
		WHERE id = $1 AND processing_status IN ('RECEIVED', 'FAILED')
	`, at)
}

func (r *WhatsAppWebhookEventRepository) MarkProcessed(ctx context.Context, id string, at time.Time) error {
	return r.transition(ctx, id, `
		UPDATE whatsapp_webhook_events
		SET processing_status = 'PROCESSED', processed_at = $2, updated_at = $2
		WHERE id = $1 AND processing_status = 'PROCESSING'
	`, at)
}

func (r *WhatsAppWebhookEventRepository) MarkFailed(ctx context.Context, id string, at time.Time, code, message string) error {
	message = strings.TrimSpace(message)
	if len(message) > 512 {
		message = message[:512]
	}
	tag, err := r.db.Exec(ctx, `
		UPDATE whatsapp_webhook_events
		SET processing_status = 'FAILED', processed_at = $2,
			last_error_code = $3, last_error_message = $4, updated_at = $2
		WHERE id = $1 AND processing_status = 'PROCESSING'
	`, id, at, nullableString(code), nullableString(message))
	if err != nil {
		return fmt.Errorf("mark WhatsApp webhook failed: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return errors.New("WhatsApp webhook event is not processing")
	}
	return nil
}

func (r *WhatsAppWebhookEventRepository) transition(ctx context.Context, id, statement string, at time.Time) error {
	tag, err := r.db.Exec(ctx, statement, id, at)
	if err != nil {
		return fmt.Errorf("transition WhatsApp webhook event: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return errors.New("WhatsApp webhook event lifecycle conflict")
	}
	return nil
}

func nullableString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

const lockWhatsAppMessageSQL = `
SELECT processing_status, resolved_user_id::text
FROM whatsapp_messages
WHERE provider_message_id = $1
FOR UPDATE
`

const completeIdentityResolutionSQL = `
UPDATE whatsapp_messages
SET processing_status = $2,
    resolved_user_id = $3
WHERE provider_message_id = $1
`

func (r *WhatsAppMessageRepository) CompleteIdentityResolution(
	ctx context.Context,
	providerMessageID string,
	resolve func(context.Context, any) (identity.Resolution, error),
) (resolution identity.Resolution, processed bool, err error) {
	if r.begin == nil {
		return identity.Resolution{}, false, errors.New("message processing transaction is unavailable")
	}

	tx, err := r.begin(ctx)
	if err != nil {
		return identity.Resolution{}, false, fmt.Errorf("begin WhatsApp message processing: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	var processingStatus string
	var resolvedUserID *string
	err = tx.QueryRow(ctx, lockWhatsAppMessageSQL, providerMessageID).Scan(
		&processingStatus,
		&resolvedUserID,
	)
	if err != nil {
		return identity.Resolution{}, false, fmt.Errorf("lock WhatsApp message: %w", err)
	}

	if processingStatus == ingestion.ProcessingStatusIdentityResolved {
		resolution = resolutionFromStoredUserID(resolvedUserID)
		if err := tx.Commit(ctx); err != nil {
			return identity.Resolution{}, false, fmt.Errorf("commit completed WhatsApp message lookup: %w", err)
		}
		return resolution, false, nil
	}
	if processingStatus != ingestion.ProcessingStatusReceived {
		return identity.Resolution{}, false, fmt.Errorf(
			"unsupported WhatsApp message processing status %q",
			processingStatus,
		)
	}

	resolution, err = resolve(ctx, tx)
	if err != nil {
		return identity.Resolution{}, false, err
	}
	if resolution.Registered && resolution.UserID == "" {
		return identity.Resolution{}, false, errors.New("registered identity is missing user ID")
	}
	if !resolution.Registered && resolution.UserID != "" {
		return identity.Resolution{}, false, errors.New("unknown identity must not include user ID")
	}

	var userID any
	if resolution.Registered {
		userID = resolution.UserID
	}
	if _, err := tx.Exec(
		ctx,
		completeIdentityResolutionSQL,
		providerMessageID,
		ingestion.ProcessingStatusIdentityResolved,
		userID,
	); err != nil {
		return identity.Resolution{}, false, fmt.Errorf("complete WhatsApp identity resolution: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return identity.Resolution{}, false, fmt.Errorf("commit WhatsApp identity resolution: %w", err)
	}

	return resolution, true, nil
}

func resolutionFromStoredUserID(userID *string) identity.Resolution {
	if userID == nil {
		return identity.Resolution{Registered: false}
	}
	return identity.Resolution{Registered: true, UserID: *userID}
}
