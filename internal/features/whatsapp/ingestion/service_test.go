package ingestion

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/harzeezco/zeexco_commerce/internal/features/identity"
	"github.com/harzeezco/zeexco_commerce/internal/features/whatsapp/models"
)

func TestIngestWebhookPreservesPayloadMetadataTraceabilityAndLifecycle(t *testing.T) {
	messages := newMemoryMessageRepository()
	events := &memoryWebhookEventRepository{}
	service := newTestService(t, messages, newFakeIdentityResolver(nil))
	service.SetWebhookEventRepository(events)
	payload := []byte(`{"object":"whatsapp_business_account","entry":[{"id":"waba-1","changes":[{"field":"messages","value":{"metadata":{"phone_number_id":"phone-1"},"messages":[{"id":"message-1","from":"sender-1","type":"text","text":{"body":"hello"}},{"id":"message-2","from":"sender-1","type":"text","text":{"body":"again"}}]}}]}]}`)

	result, err := service.IngestWebhook(context.Background(), payload)
	if err != nil {
		t.Fatal(err)
	}
	if result.EventID != "event-1" || len(result.Messages) != 2 || len(result.Results) != 2 {
		t.Fatalf("result = %#v", result)
	}
	if !bytes.Equal(events.event.Payload, payload) || events.event.ProviderAccountID != "waba-1" ||
		events.event.PhoneNumberID != "phone-1" || events.event.EventType != "messages" ||
		events.status != WebhookStatusProcessed || events.attempts != 1 {
		t.Fatalf("event = %#v status=%q attempts=%d", events.event, events.status, events.attempts)
	}
	for _, id := range []string{"message-1", "message-2"} {
		if messages.records[id].SourceWebhookEventID != "event-1" {
			t.Fatalf("message %s source = %q", id, messages.records[id].SourceWebhookEventID)
		}
	}
}

func TestIngestWebhookPersistsThenMarksMalformedMessageFailed(t *testing.T) {
	events := &memoryWebhookEventRepository{}
	service := newTestService(t, newMemoryMessageRepository(), newFakeIdentityResolver(nil))
	service.SetWebhookEventRepository(events)
	payload := []byte(`{"object":"whatsapp_business_account","entry":[{"id":"waba-1","changes":[{"field":"messages","value":{"messages":[{"from":"sender","type":"text","text":{"body":"hello"}}]}}]}]}`)

	_, err := service.IngestWebhook(context.Background(), payload)
	var invalid *InvalidWebhookError
	if !errors.As(err, &invalid) || events.status != WebhookStatusFailed || events.errorCode != "INVALID_PAYLOAD" {
		t.Fatalf("error=%v status=%q code=%q", err, events.status, events.errorCode)
	}
}

func TestIngestWebhookReturnsDurableStorageFailure(t *testing.T) {
	events := &memoryWebhookEventRepository{err: errors.New("database unavailable")}
	service := newTestService(t, newMemoryMessageRepository(), newFakeIdentityResolver(nil))
	service.SetWebhookEventRepository(events)
	_, err := service.IngestWebhook(context.Background(), []byte(`{"object":"whatsapp_business_account"}`))
	if !errors.Is(err, events.err) {
		t.Fatalf("error = %v", err)
	}
}

func TestAcceptWebhookPersistsWithoutDownstreamProcessing(t *testing.T) {
	messages := newMemoryMessageRepository()
	events := &memoryWebhookEventRepository{}
	service := newTestService(t, messages, newFakeIdentityResolver(nil))
	service.SetWebhookEventRepository(events)
	payload := []byte(`{"object":"whatsapp_business_account","entry":[]}`)
	result, err := service.AcceptWebhook(context.Background(), payload)
	if err != nil {
		t.Fatal(err)
	}
	if result.EventID != "event-1" || events.status != WebhookStatusReceived || events.attempts != 0 || messages.count() != 0 {
		t.Fatalf("result=%#v status=%q attempts=%d messages=%d", result, events.status, events.attempts, messages.count())
	}
}

func TestProcessNextWebhookClaimsAndCompletesStoredEvent(t *testing.T) {
	messages := newMemoryMessageRepository()
	events := &memoryWebhookEventRepository{
		claim: true,
		claimEvent: models.WebhookEvent{
			ID: "event-1", Payload: []byte(`{"object":"whatsapp_business_account","entry":[]}`),
		},
	}
	service := newTestService(t, messages, newFakeIdentityResolver(nil))
	service.SetWebhookEventRepository(events)
	claimed, err := service.ProcessNextWebhook(context.Background(), time.Now(), time.Now(), 5)
	if err != nil || !claimed || events.status != WebhookStatusProcessed {
		t.Fatalf("claimed=%t status=%q error=%v", claimed, events.status, err)
	}
}

type memoryWebhookEventRepository struct {
	event      models.WebhookEvent
	status     string
	attempts   int
	errorCode  string
	err        error
	claim      bool
	claimEvent models.WebhookEvent
}

func (r *memoryWebhookEventRepository) Store(_ context.Context, event models.WebhookEvent) (models.WebhookEvent, error) {
	if r.err != nil {
		return models.WebhookEvent{}, r.err
	}
	event.ID = "event-1"
	r.event = event
	r.status = event.ProcessingStatus
	return event, nil
}
func (r *memoryWebhookEventRepository) ClaimNext(context.Context, time.Time, time.Time, time.Time, int) (models.WebhookEvent, bool, error) {
	if !r.claim {
		return models.WebhookEvent{}, false, nil
	}
	r.claim = false
	r.status = WebhookStatusProcessing
	r.attempts++
	return r.claimEvent, true, nil
}
func (r *memoryWebhookEventRepository) MarkProcessing(context.Context, string, time.Time) error {
	r.status = WebhookStatusProcessing
	r.attempts++
	return nil
}
func (r *memoryWebhookEventRepository) MarkProcessed(context.Context, string, time.Time) error {
	r.status = WebhookStatusProcessed
	return nil
}
func (r *memoryWebhookEventRepository) MarkFailed(_ context.Context, _ string, _ time.Time, code, _ string) error {
	r.status = WebhookStatusFailed
	r.errorCode = code
	return nil
}

func TestServiceStoresNewMessageAndResolvesRegisteredIdentity(t *testing.T) {
	repository := newMemoryMessageRepository()
	resolver := newFakeIdentityResolver(map[string]identity.Resolution{
		"sender-1": {Registered: true, UserID: "user-id-1"},
	})
	service := newTestService(t, repository, resolver)

	results, err := service.Ingest(context.Background(), []models.IncomingMessage{
		incomingMessage("message-1", "sender-1"),
	})
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}

	if len(results) != 1 ||
		results[0].Status != StatusNew ||
		!results[0].Identity.Registered ||
		results[0].Identity.UserID != "user-id-1" {
		t.Fatalf("results = %#v", results)
	}
	if repository.count() != 1 {
		t.Fatalf("stored records = %d, want 1", repository.count())
	}
	if resolver.resolveCalls != 1 || resolver.writeCalls != 0 {
		t.Fatalf("resolve calls = %d, writes = %d", resolver.resolveCalls, resolver.writeCalls)
	}
}

func TestServiceStoresUnknownSenderWithoutCreatingIdentityOrUser(t *testing.T) {
	repository := newMemoryMessageRepository()
	resolver := newFakeIdentityResolver(nil)
	service := newTestService(t, repository, resolver)

	results, err := service.Ingest(context.Background(), []models.IncomingMessage{
		incomingMessage("message-1", "unknown-sender"),
	})
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}

	if len(results) != 1 || results[0].Status != StatusNew || results[0].Identity.Registered {
		t.Fatalf("results = %#v", results)
	}
	if repository.count() != 1 {
		t.Fatalf("stored records = %d, want 1", repository.count())
	}
	if resolver.resolveCalls != 1 || resolver.writeCalls != 0 {
		t.Fatalf("resolve calls = %d, writes = %d", resolver.resolveCalls, resolver.writeCalls)
	}
}

func TestServiceDeduplicatesAndContinuesOtherMessages(t *testing.T) {
	repository := newMemoryMessageRepository()
	completedMessage := incomingStorageMessage("message-b")
	completedMessage.ProcessingStatus = ProcessingStatusIdentityResolved
	repository.records["message-b"] = completedMessage
	repository.resolutions["message-b"] = identity.Resolution{Registered: false}
	resolver := newFakeIdentityResolver(nil)
	service := newTestService(t, repository, resolver)

	results, err := service.Ingest(context.Background(), []models.IncomingMessage{
		incomingMessage("message-a", "sender-a"),
		incomingMessage("message-b", "sender-b"),
		incomingMessage("message-c", "sender-c"),
	})
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}

	wantStatuses := []Status{StatusNew, StatusDuplicate, StatusNew}
	if len(results) != len(wantStatuses) {
		t.Fatalf("result count = %d, want %d", len(results), len(wantStatuses))
	}
	for index, wantStatus := range wantStatuses {
		if results[index].Status != wantStatus {
			t.Errorf("result %d status = %s, want %s", index, results[index].Status, wantStatus)
		}
	}

	if repository.count() != 3 {
		t.Fatalf("stored records = %d, want 3", repository.count())
	}
	if resolver.resolveCalls != 2 {
		t.Fatalf("identity resolutions = %d, want 2", resolver.resolveCalls)
	}
}

func TestServiceDuplicateResumesIncompleteIdentityResolution(t *testing.T) {
	repository := newMemoryMessageRepository()
	resolver := &failOnceIdentityResolver{
		resolution: identity.Resolution{Registered: true, UserID: "user-id-1"},
	}
	service := newTestService(t, repository, resolver)
	message := incomingMessage("message-1", "sender-1")

	if _, err := service.Ingest(context.Background(), []models.IncomingMessage{message}); err == nil {
		t.Fatal("first Ingest() error = nil, want temporary identity failure")
	}
	if repository.count() != 1 {
		t.Fatalf("stored records after failure = %d, want 1", repository.count())
	}
	if repository.records[message.ID].ProcessingStatus != ProcessingStatusReceived {
		t.Fatalf("status after failure = %q, want %q", repository.records[message.ID].ProcessingStatus, ProcessingStatusReceived)
	}

	results, err := service.Ingest(context.Background(), []models.IncomingMessage{message})
	if err != nil {
		t.Fatalf("retry Ingest() error = %v", err)
	}
	if results[0].Status != StatusDuplicate || !results[0].Identity.Registered {
		t.Fatalf("retry result = %#v", results[0])
	}
	if repository.records[message.ID].ProcessingStatus != ProcessingStatusIdentityResolved {
		t.Fatalf("status after retry = %q, want %q", repository.records[message.ID].ProcessingStatus, ProcessingStatusIdentityResolved)
	}

	if _, err := service.Ingest(context.Background(), []models.IncomingMessage{message}); err != nil {
		t.Fatalf("completed duplicate Ingest() error = %v", err)
	}
	if resolver.calls != 2 {
		t.Fatalf("identity resolution calls = %d, want 2 (failed attempt plus successful retry)", resolver.calls)
	}
}

func TestServiceStoresDifferentProviderMessageIDsSeparately(t *testing.T) {
	repository := newMemoryMessageRepository()
	service := newTestService(t, repository, newFakeIdentityResolver(nil))

	results, err := service.Ingest(context.Background(), []models.IncomingMessage{
		incomingMessage("message-1", "sender-1"),
		incomingMessage("message-2", "sender-1"),
	})
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}
	if results[0].Status != StatusNew || results[1].Status != StatusNew {
		t.Fatalf("results = %#v", results)
	}
	if repository.count() != 2 {
		t.Fatalf("stored records = %d, want 2", repository.count())
	}
}

func TestServiceStoresNormalizedContentForSupportedMessageTypes(t *testing.T) {
	repository := newMemoryMessageRepository()
	service := newTestService(t, repository, newFakeIdentityResolver(nil))

	messages := []models.IncomingMessage{
		incomingMessage("text-message", "sender-1"),
		{
			ID:                    "button-message",
			SenderWhatsAppID:      "sender-1",
			BusinessPhoneNumberID: "business-phone-id-1",
			Type:                  "interactive",
			Supported:             true,
			InteractiveButton: &models.IncomingReply{
				ID:    "button-id",
				Title: "Button title",
			},
		},
		{
			ID:                    "list-message",
			SenderWhatsAppID:      "sender-1",
			BusinessPhoneNumberID: "business-phone-id-1",
			Type:                  "interactive",
			Supported:             true,
			InteractiveList: &models.IncomingReply{
				ID:    "list-id",
				Title: "List title",
			},
		},
		{
			ID:                    "template-button-message",
			SenderWhatsAppID:      "sender-1",
			BusinessPhoneNumberID: "business-phone-id-1",
			Type:                  "button",
			Supported:             true,
			TemplateButton: &models.IncomingButton{
				Payload: "template-payload",
				Text:    "Template text",
			},
		},
		{
			ID:                    "unsupported-message",
			SenderWhatsAppID:      "sender-1",
			BusinessPhoneNumberID: "business-phone-id-1",
			Type:                  "image",
		},
	}

	if _, err := service.Ingest(context.Background(), messages); err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}

	if repository.records["text-message"].Content.Text != "sanitized test message" {
		t.Fatalf("text content = %#v", repository.records["text-message"].Content)
	}
	buttonContent := repository.records["button-message"].Content
	if buttonContent.ButtonID != "button-id" || buttonContent.ButtonTitle != "Button title" {
		t.Fatalf("button content = %#v", buttonContent)
	}
	listContent := repository.records["list-message"].Content
	if listContent.ListID != "list-id" || listContent.ListTitle != "List title" {
		t.Fatalf("list content = %#v", listContent)
	}
	templateContent := repository.records["template-button-message"].Content
	if templateContent.TemplateButtonPayload != "template-payload" ||
		templateContent.TemplateButtonText != "Template text" {
		t.Fatalf("template button content = %#v", templateContent)
	}
	if repository.records["unsupported-message"].Content != (Content{}) {
		t.Fatalf("unsupported content = %#v, want empty normalized content", repository.records["unsupported-message"].Content)
	}
}

func TestServiceConcurrentDuplicateHasSingleNewResult(t *testing.T) {
	repository := newMemoryMessageRepository()
	service := newTestService(t, repository, newFakeIdentityResolver(nil))

	const deliveries = 20
	statuses := make(chan Status, deliveries)
	errorsChannel := make(chan error, deliveries)
	var waitGroup sync.WaitGroup

	for range deliveries {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			results, err := service.Ingest(context.Background(), []models.IncomingMessage{
				incomingMessage("same-message", "sender-1"),
			})
			if err != nil {
				errorsChannel <- err
				return
			}
			statuses <- results[0].Status
		}()
	}

	waitGroup.Wait()
	close(statuses)
	close(errorsChannel)

	for err := range errorsChannel {
		t.Fatalf("Ingest() concurrent error = %v", err)
	}

	newCount := 0
	duplicateCount := 0
	for status := range statuses {
		switch status {
		case StatusNew:
			newCount++
		case StatusDuplicate:
			duplicateCount++
		}
	}

	if newCount != 1 || duplicateCount != deliveries-1 || repository.count() != 1 {
		t.Fatalf(
			"new = %d, duplicate = %d, records = %d",
			newCount,
			duplicateCount,
			repository.count(),
		)
	}
}

func TestServiceReturnsRepositoryFailure(t *testing.T) {
	repository := newMemoryMessageRepository()
	repository.err = errors.New("simulated database failure")
	service := newTestService(t, repository, newFakeIdentityResolver(nil))

	_, err := service.Ingest(context.Background(), []models.IncomingMessage{
		incomingMessage("message-1", "sender-1"),
	})
	if err == nil {
		t.Fatal("Ingest() error = nil, want repository error")
	}
}

func newTestService(
	t *testing.T,
	repository MessageRepository,
	resolver IdentityResolver,
) *Service {
	t.Helper()

	service, err := NewService(
		repository,
		resolver,
		func() time.Time { return time.Unix(1800000000, 0).UTC() },
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return service
}

func incomingMessage(messageID, senderID string) models.IncomingMessage {
	timestamp := time.Unix(1700000000, 0).UTC()
	return models.IncomingMessage{
		ID:                    messageID,
		SenderWhatsAppID:      senderID,
		BusinessPhoneNumberID: "business-phone-id-1",
		Timestamp:             &timestamp,
		Type:                  "text",
		Supported:             true,
		Text:                  "sanitized test message",
	}
}

func incomingStorageMessage(messageID string) Message {
	return Message{
		ProviderMessageID:     messageID,
		SenderProviderID:      "sender",
		BusinessPhoneNumberID: "business-phone-id-1",
		MessageType:           "text",
		Content:               Content{Text: "test"},
		ProcessingStatus:      ProcessingStatusReceived,
		ReceivedAt:            time.Unix(1800000000, 0).UTC(),
	}
}

type memoryMessageRepository struct {
	mu          sync.Mutex
	records     map[string]Message
	resolutions map[string]identity.Resolution
	err         error
}

func newMemoryMessageRepository() *memoryMessageRepository {
	return &memoryMessageRepository{
		records:     make(map[string]Message),
		resolutions: make(map[string]identity.Resolution),
	}
}

func (r *memoryMessageRepository) Store(
	_ context.Context,
	message Message,
) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.err != nil {
		return false, r.err
	}
	if _, exists := r.records[message.ProviderMessageID]; exists {
		return false, nil
	}
	r.records[message.ProviderMessageID] = message
	return true, nil
}

func (r *memoryMessageRepository) CompleteIdentityResolution(
	ctx context.Context,
	providerMessageID string,
	resolve func(context.Context, any) (identity.Resolution, error),
) (identity.Resolution, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	message, exists := r.records[providerMessageID]
	if !exists {
		return identity.Resolution{}, false, errors.New("message not found")
	}
	if message.ProcessingStatus == ProcessingStatusIdentityResolved {
		return r.resolutions[providerMessageID], false, nil
	}

	resolution, err := resolve(ctx, nil)
	if err != nil {
		return identity.Resolution{}, false, err
	}
	message.ProcessingStatus = ProcessingStatusIdentityResolved
	r.records[providerMessageID] = message
	r.resolutions[providerMessageID] = resolution
	return resolution, true, nil
}

func (r *memoryMessageRepository) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.records)
}

type fakeIdentityResolver struct {
	mu           sync.Mutex
	resolutions  map[string]identity.Resolution
	resolveCalls int
	writeCalls   int
}

type failOnceIdentityResolver struct {
	mu         sync.Mutex
	calls      int
	resolution identity.Resolution
}

func (r *failOnceIdentityResolver) ResolveIdentity(
	_ context.Context,
	_ identity.Provider,
	_ string,
) (identity.Resolution, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	if r.calls == 1 {
		return identity.Resolution{}, errors.New("temporary identity lookup failure")
	}
	return r.resolution, nil
}

func (r *failOnceIdentityResolver) ResolveIdentityWithScope(
	ctx context.Context,
	_ any,
	provider identity.Provider,
	providerUserID string,
) (identity.Resolution, error) {
	return r.ResolveIdentity(ctx, provider, providerUserID)
}

func newFakeIdentityResolver(
	resolutions map[string]identity.Resolution,
) *fakeIdentityResolver {
	return &fakeIdentityResolver{resolutions: resolutions}
}

func (r *fakeIdentityResolver) ResolveIdentity(
	_ context.Context,
	provider identity.Provider,
	providerUserID string,
) (identity.Resolution, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if provider != identity.ProviderWhatsApp {
		return identity.Resolution{}, errors.New("unexpected provider")
	}
	r.resolveCalls++
	return r.resolutions[providerUserID], nil
}

func (r *fakeIdentityResolver) ResolveIdentityWithScope(
	ctx context.Context,
	_ any,
	provider identity.Provider,
	providerUserID string,
) (identity.Resolution, error) {
	return r.ResolveIdentity(ctx, provider, providerUserID)
}
