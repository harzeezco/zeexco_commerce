package ingestion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/harzeezco/zeexco_commerce/internal/features/identity"
	"github.com/harzeezco/zeexco_commerce/internal/features/whatsapp/models"
	"github.com/harzeezco/zeexco_commerce/internal/features/whatsapp/parser"
)

type Status string

const (
	StatusNew       Status = "NEW"
	StatusDuplicate Status = "DUPLICATE"

	ProcessingStatusReceived         = "RECEIVED"
	ProcessingStatusIdentityResolved = "IDENTITY_RESOLVED"

	WebhookStatusReceived   = "RECEIVED"
	WebhookStatusProcessing = "PROCESSING"
	WebhookStatusProcessed  = "PROCESSED"
	WebhookStatusFailed     = "FAILED"
	WebhookProviderMeta     = "META_WHATSAPP"
)

var (
	ErrMessageRepositoryRequired = errors.New("message repository is required")
	ErrIdentityResolverRequired  = errors.New("identity resolver is required")
)

type Message struct {
	ProviderMessageID     string
	SenderProviderID      string
	BusinessPhoneNumberID string
	MessageType           string
	Content               Content
	ProcessingStatus      string
	ProviderTimestamp     *time.Time
	ReceivedAt            time.Time
	SourceWebhookEventID  string
}

type Content struct {
	Text                  string `json:"text,omitempty"`
	ButtonID              string `json:"button_id,omitempty"`
	ButtonTitle           string `json:"button_title,omitempty"`
	ListID                string `json:"list_id,omitempty"`
	ListTitle             string `json:"list_title,omitempty"`
	TemplateButtonPayload string `json:"template_button_payload,omitempty"`
	TemplateButtonText    string `json:"template_button_text,omitempty"`
}

type Result struct {
	Status   Status
	Identity identity.Resolution
}

type WebhookResult struct {
	EventID  string
	Object   string
	Messages []models.IncomingMessage
	Results  []Result
}

type WebhookEventRepository interface {
	Store(context.Context, models.WebhookEvent) (models.WebhookEvent, error)
	ClaimNext(context.Context, time.Time, time.Time, time.Time, int) (models.WebhookEvent, bool, error)
	MarkProcessing(context.Context, string, time.Time) error
	MarkProcessed(context.Context, string, time.Time) error
	MarkFailed(context.Context, string, time.Time, string, string) error
}

type InvalidWebhookError struct{ Err error }

func (e *InvalidWebhookError) Error() string { return "invalid WhatsApp webhook" }
func (e *InvalidWebhookError) Unwrap() error { return e.Err }

type MessageRepository interface {
	Store(ctx context.Context, message Message) (stored bool, err error)
	CompleteIdentityResolution(
		ctx context.Context,
		providerMessageID string,
		resolve func(context.Context, any) (identity.Resolution, error),
	) (resolution identity.Resolution, processed bool, err error)
}

type IdentityResolver interface {
	ResolveIdentity(
		ctx context.Context,
		provider identity.Provider,
		providerUserID string,
	) (identity.Resolution, error)
	ResolveIdentityWithScope(
		ctx context.Context,
		scope any,
		provider identity.Provider,
		providerUserID string,
	) (identity.Resolution, error)
}

type IdentityOutcomeHandler interface {
	HandleIdentityOutcome(
		context.Context,
		models.IncomingMessage,
		identity.Resolution,
		bool,
	) error
}

type Service struct {
	messages   MessageRepository
	identities IdentityResolver
	now        func() time.Time
	outcomes   IdentityOutcomeHandler
	events     WebhookEventRepository
}

func (s *Service) SetWebhookEventRepository(repository WebhookEventRepository) {
	s.events = repository
}

func (s *Service) IngestWebhook(ctx context.Context, payload []byte) (WebhookResult, error) {
	result, err := s.AcceptWebhook(ctx, payload)
	if err != nil {
		return WebhookResult{}, err
	}
	if err := s.events.MarkProcessing(ctx, result.EventID, s.now().UTC()); err != nil {
		return result, fmt.Errorf("start WhatsApp webhook processing: %w", err)
	}
	event := models.WebhookEvent{ID: result.EventID, Payload: append([]byte(nil), payload...)}
	return s.processWebhookEvent(ctx, event, result.Object)
}

// AcceptWebhook durably stores a verified provider delivery without executing
// downstream message or onboarding work. The caller can acknowledge Meta as
// soon as this method succeeds; a worker claims the stored event afterward.
func (s *Service) AcceptWebhook(ctx context.Context, payload []byte) (WebhookResult, error) {
	if s.events == nil {
		return WebhookResult{}, errors.New("webhook event repository is required")
	}
	var envelope models.WebhookEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil || envelope.Object == "" {
		return WebhookResult{}, &InvalidWebhookError{Err: errors.New("invalid webhook envelope")}
	}
	now := s.now().UTC()
	accountID, phoneNumberID, eventType := webhookMetadata(envelope)
	event, err := s.events.Store(ctx, models.WebhookEvent{
		Provider: WebhookProviderMeta, ProviderAccountID: accountID,
		PhoneNumberID: phoneNumberID, EventType: eventType,
		Payload: append([]byte(nil), payload...), ProcessingStatus: WebhookStatusReceived,
		ReceivedAt: now,
	})
	if err != nil {
		return WebhookResult{}, fmt.Errorf("store WhatsApp webhook event: %w", err)
	}
	return WebhookResult{EventID: event.ID, Object: envelope.Object}, nil
}

func (s *Service) ProcessNextWebhook(
	ctx context.Context,
	retryBefore time.Time,
	staleBefore time.Time,
	maxAttempts int,
) (bool, error) {
	if s.events == nil {
		return false, errors.New("webhook event repository is required")
	}
	event, claimed, err := s.events.ClaimNext(ctx, s.now().UTC(), retryBefore, staleBefore, maxAttempts)
	if err != nil || !claimed {
		return claimed, err
	}
	var envelope models.WebhookEnvelope
	if err := json.Unmarshal(event.Payload, &envelope); err != nil || envelope.Object == "" {
		_ = s.events.MarkFailed(context.Background(), event.ID, s.now().UTC(), "INVALID_ENVELOPE", "stored webhook envelope is invalid")
		return true, &InvalidWebhookError{Err: errors.New("invalid stored webhook envelope")}
	}
	_, err = s.processWebhookEvent(ctx, event, envelope.Object)
	return true, err
}

func (s *Service) processWebhookEvent(ctx context.Context, event models.WebhookEvent, object string) (WebhookResult, error) {
	result := WebhookResult{EventID: event.ID, Object: object}
	if object != "whatsapp_business_account" {
		if err := s.events.MarkProcessed(ctx, event.ID, s.now().UTC()); err != nil {
			return result, fmt.Errorf("complete unsupported WhatsApp webhook: %w", err)
		}
		return result, nil
	}
	messages, err := parser.ParseMessages(event.Payload)
	if err != nil {
		_ = s.events.MarkFailed(context.Background(), event.ID, s.now().UTC(), "INVALID_PAYLOAD", "webhook message parsing failed")
		return result, &InvalidWebhookError{Err: err}
	}
	result.Messages = messages
	result.Results, err = s.ingest(ctx, messages, event.ID)
	if err != nil {
		_ = s.events.MarkFailed(context.Background(), event.ID, s.now().UTC(), "PROCESSING_FAILED", "webhook processing failed")
		return result, err
	}
	if err := s.events.MarkProcessed(ctx, event.ID, s.now().UTC()); err != nil {
		return result, fmt.Errorf("complete WhatsApp webhook event: %w", err)
	}
	return result, nil
}

func webhookMetadata(envelope models.WebhookEnvelope) (accountID, phoneNumberID, eventType string) {
	eventType = "unknown"
	for _, entry := range envelope.Entry {
		if accountID == "" {
			accountID = entry.ID
		}
		for _, change := range entry.Changes {
			if eventType == "unknown" && strings.TrimSpace(change.Field) != "" {
				eventType = change.Field
			}
			if phoneNumberID == "" && change.Value != nil {
				phoneNumberID = change.Value.Metadata.PhoneNumberID
			}
		}
	}
	return
}

func (s *Service) SetIdentityOutcomeHandler(handler IdentityOutcomeHandler) {
	s.outcomes = handler
}

func NewService(
	messages MessageRepository,
	identities IdentityResolver,
	now func() time.Time,
) (*Service, error) {
	if messages == nil {
		return nil, ErrMessageRepositoryRequired
	}
	if identities == nil {
		return nil, ErrIdentityResolverRequired
	}
	if now == nil {
		now = time.Now
	}

	return &Service{
		messages:   messages,
		identities: identities,
		now:        now,
	}, nil
}

func (s *Service) Ingest(
	ctx context.Context,
	messages []models.IncomingMessage,
) ([]Result, error) {
	return s.ingest(ctx, messages, "")
}

func (s *Service) ingest(ctx context.Context, messages []models.IncomingMessage, sourceEventID string) ([]Result, error) {
	results := make([]Result, 0, len(messages))
	receivedAt := s.now().UTC()

	for index, message := range messages {
		storedMessage := messageForStorage(message, receivedAt)
		storedMessage.SourceWebhookEventID = sourceEventID
		stored, err := s.messages.Store(ctx, storedMessage)
		if err != nil {
			return results, fmt.Errorf("store WhatsApp message at index %d: %w", index, err)
		}

		resolution, processed, err := s.messages.CompleteIdentityResolution(
			ctx,
			message.ID,
			func(processingContext context.Context, scope any) (identity.Resolution, error) {
				if scope == nil {
					return s.identities.ResolveIdentity(
						processingContext,
						identity.ProviderWhatsApp,
						message.SenderWhatsAppID,
					)
				}

				return s.identities.ResolveIdentityWithScope(
					processingContext,
					scope,
					identity.ProviderWhatsApp,
					message.SenderWhatsAppID,
				)
			},
		)
		if err != nil {
			return results, fmt.Errorf("resolve sender identity at index %d: %w", index, err)
		}
		if s.outcomes != nil {
			if err := s.outcomes.HandleIdentityOutcome(ctx, message, resolution, processed); err != nil {
				return results, fmt.Errorf("handle sender outcome at index %d: %w", index, err)
			}
		}

		results = append(results, Result{
			Status:   deliveryStatus(stored),
			Identity: resolution,
		})
	}

	return results, nil
}

func deliveryStatus(stored bool) Status {
	if stored {
		return StatusNew
	}
	return StatusDuplicate
}

func messageForStorage(message models.IncomingMessage, receivedAt time.Time) Message {
	content := Content{Text: message.Text}
	if message.InteractiveButton != nil {
		content.ButtonID = message.InteractiveButton.ID
		content.ButtonTitle = message.InteractiveButton.Title
	}
	if message.InteractiveList != nil {
		content.ListID = message.InteractiveList.ID
		content.ListTitle = message.InteractiveList.Title
	}
	if message.TemplateButton != nil {
		content.TemplateButtonPayload = message.TemplateButton.Payload
		content.TemplateButtonText = message.TemplateButton.Text
	}

	return Message{
		ProviderMessageID:     message.ID,
		SenderProviderID:      message.SenderWhatsAppID,
		BusinessPhoneNumberID: message.BusinessPhoneNumberID,
		MessageType:           message.Type,
		Content:               content,
		ProcessingStatus:      ProcessingStatusReceived,
		ProviderTimestamp:     message.Timestamp,
		ReceivedAt:            receivedAt,
	}
}
