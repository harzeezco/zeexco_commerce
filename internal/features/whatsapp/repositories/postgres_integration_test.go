//go:build integration

package repositories

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/harzeezco/zeexco_commerce/internal/features/identity"
	"github.com/harzeezco/zeexco_commerce/internal/features/whatsapp/ingestion"
	"github.com/harzeezco/zeexco_commerce/internal/features/whatsapp/models"
	"github.com/harzeezco/zeexco_commerce/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresWhatsAppFoundation(t *testing.T) {
	ctx := context.Background()
	pool := migratedTestPool(t, ctx)

	eventRepository := NewWhatsAppWebhookEventRepository(pool)
	rawPayload := []byte(`{"object":"whatsapp_business_account","entry":[{"id":"waba-integration"}]}`)
	event, err := eventRepository.Store(ctx, models.WebhookEvent{
		Provider: ingestion.WebhookProviderMeta, ProviderAccountID: "waba-integration",
		PhoneNumberID: "phone-integration", EventType: "messages", Payload: rawPayload,
		ProcessingStatus: ingestion.WebhookStatusReceived, ReceivedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := eventRepository.MarkProcessing(ctx, event.ID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := eventRepository.MarkProcessed(ctx, event.ID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	var payload []byte
	var eventStatus string
	var attempts int
	if err := pool.QueryRow(ctx, `SELECT payload, processing_status, attempt_count FROM whatsapp_webhook_events WHERE id=$1`, event.ID).Scan(&payload, &eventStatus, &attempts); err != nil {
		t.Fatal(err)
	}
	if !jsonEqual(payload, rawPayload) || eventStatus != ingestion.WebhookStatusProcessed || attempts != 1 {
		t.Fatalf("payload=%s status=%s attempts=%d", payload, eventStatus, attempts)
	}

	var userID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (first_name, last_name, email)
		VALUES ('Integration', 'User', 'integration@example.com')
		RETURNING id::text
	`).Scan(&userID); err != nil {
		t.Fatalf("insert test user: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO whatsapp_identities (user_id, provider, provider_user_id)
		VALUES ($1, $2, $3)
	`, userID, metaWhatsAppProvider, "known-sender"); err != nil {
		t.Fatalf("insert test identity: %v", err)
	}

	identityRepository := NewWhatsAppIdentityRepository(pool)
	known, err := identityRepository.Resolve(ctx, "known-sender")
	if err != nil || !known.Registered || known.UserID != userID {
		t.Fatalf("known identity = %#v, error = %v", known, err)
	}

	usersBefore := tableCount(t, ctx, pool, "users")
	identitiesBefore := tableCount(t, ctx, pool, "whatsapp_identities")
	unknown, err := identityRepository.Resolve(ctx, "unknown-sender")
	if err != nil || unknown.Registered {
		t.Fatalf("unknown identity = %#v, error = %v", unknown, err)
	}
	if tableCount(t, ctx, pool, "users") != usersBefore ||
		tableCount(t, ctx, pool, "whatsapp_identities") != identitiesBefore {
		t.Fatal("unknown identity lookup created database records")
	}

	messageRepository := NewWhatsAppMessageRepository(pool)
	message := integrationStoredMessage("sequential-duplicate", "known-sender")
	message.SourceWebhookEventID = event.ID
	firstStored, err := messageRepository.Store(ctx, message)
	if err != nil || !firstStored {
		t.Fatalf("first Store() stored = %v, error = %v", firstStored, err)
	}
	secondStored, err := messageRepository.Store(ctx, message)
	if err != nil || secondStored {
		t.Fatalf("duplicate Store() stored = %v, error = %v", secondStored, err)
	}
	if countMessage(t, ctx, pool, message.ProviderMessageID) != 1 {
		t.Fatal("sequential duplicate created more than one row")
	}
	second := integrationStoredMessage("same-source-second-message", "known-sender")
	second.SourceWebhookEventID = event.ID
	if stored, err := messageRepository.Store(ctx, second); err != nil || !stored {
		t.Fatalf("second source message stored=%v error=%v", stored, err)
	}
	var linked int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM whatsapp_messages WHERE source_webhook_event_id=$1`, event.ID).Scan(&linked); err != nil || linked != 2 {
		t.Fatalf("linked messages=%d error=%v", linked, err)
	}

	resolver := identity.NewResolver(map[identity.Provider]identity.ProviderResolver{
		identity.ProviderWhatsApp: identityRepository,
	})
	concurrentResolver := &countingResolver{resolution: identity.Resolution{Registered: false}}
	concurrentService := integrationService(t, messageRepository, concurrentResolver)
	concurrentMessage := integrationIncomingMessage("concurrent-duplicate", "unknown-sender")
	runConcurrentDeliveries(t, concurrentService, concurrentMessage, 20)
	if countMessage(t, ctx, pool, concurrentMessage.ID) != 1 {
		t.Fatal("concurrent duplicate created more than one row")
	}
	if concurrentResolver.CallCount() != 1 {
		t.Fatalf("concurrent identity processing calls = %d, want 1", concurrentResolver.CallCount())
	}
	assertMessageState(
		t,
		ctx,
		pool,
		concurrentMessage.ID,
		ingestion.ProcessingStatusIdentityResolved,
		nil,
	)

	flakyResolver := &failOnceScopedResolver{delegate: resolver}
	resumableService := integrationService(t, messageRepository, flakyResolver)
	resumableMessage := integrationIncomingMessage("resumable-message", "known-sender")
	if _, err := resumableService.Ingest(ctx, []models.IncomingMessage{resumableMessage}); err == nil {
		t.Fatal("first resumable Ingest() error = nil, want temporary failure")
	}
	assertMessageState(t, ctx, pool, resumableMessage.ID, ingestion.ProcessingStatusReceived, nil)

	results, err := resumableService.Ingest(ctx, []models.IncomingMessage{resumableMessage})
	if err != nil {
		t.Fatalf("resumable retry Ingest() error = %v", err)
	}
	if results[0].Status != ingestion.StatusDuplicate || results[0].Identity.UserID != userID {
		t.Fatalf("resumable retry result = %#v", results[0])
	}
	assertMessageState(t, ctx, pool, resumableMessage.ID, ingestion.ProcessingStatusIdentityResolved, &userID)

	if _, err := resumableService.Ingest(ctx, []models.IncomingMessage{resumableMessage}); err != nil {
		t.Fatalf("completed duplicate Ingest() error = %v", err)
	}
	if flakyResolver.CallCount() != 2 {
		t.Fatalf("resumable identity processing calls = %d, want 2", flakyResolver.CallCount())
	}
}

func jsonEqual(left, right []byte) bool {
	var a, b any
	return json.Unmarshal(left, &a) == nil && json.Unmarshal(right, &b) == nil && reflect.DeepEqual(a, b)
}

func migratedTestPool(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; start the integration PostgreSQL service and set the dedicated test URL")
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

	schema := "zeexco_it_" + randomHex(t, 8)
	schemaIdentifier := pgx.Identifier{schema}.Sanitize()
	if _, err := adminPool.Exec(ctx, "CREATE SCHEMA "+schemaIdentifier); err != nil {
		t.Fatalf("create integration schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = adminPool.Exec(context.Background(), "DROP SCHEMA "+schemaIdentifier+" CASCADE")
	})

	testConfig := adminConfig.Copy()
	testConfig.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, testConfig)
	if err != nil {
		t.Fatalf("connect to integration schema: %v", err)
	}
	t.Cleanup(pool.Close)

	applyMigrations(t, ctx, pool)
	return pool
}

func applyMigrations(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := database.Migrate(ctx, pool); err != nil {
		t.Fatalf("apply project migrations: %v", err)
	}
}

func integrationService(t *testing.T, repository ingestion.MessageRepository, resolver ingestion.IdentityResolver) *ingestion.Service {
	t.Helper()
	service, err := ingestion.NewService(repository, resolver, func() time.Time {
		return time.Unix(1800000000, 0).UTC()
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return service
}

func runConcurrentDeliveries(t *testing.T, service *ingestion.Service, message models.IncomingMessage, count int) {
	t.Helper()
	errorsChannel := make(chan error, count)
	var waitGroup sync.WaitGroup
	for range count {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			_, err := service.Ingest(context.Background(), []models.IncomingMessage{message})
			errorsChannel <- err
		}()
	}
	waitGroup.Wait()
	close(errorsChannel)
	for err := range errorsChannel {
		if err != nil {
			t.Fatalf("concurrent Ingest() error = %v", err)
		}
	}
}

func integrationStoredMessage(messageID, senderID string) ingestion.Message {
	return ingestion.Message{
		ProviderMessageID:     messageID,
		SenderProviderID:      senderID,
		BusinessPhoneNumberID: "business-phone-id",
		MessageType:           "text",
		Content:               ingestion.Content{Text: "integration test"},
		ProcessingStatus:      ingestion.ProcessingStatusReceived,
		ReceivedAt:            time.Unix(1800000000, 0).UTC(),
	}
}

func integrationIncomingMessage(messageID, senderID string) models.IncomingMessage {
	return models.IncomingMessage{
		ID:                    messageID,
		SenderWhatsAppID:      senderID,
		BusinessPhoneNumberID: "business-phone-id",
		Type:                  "text",
		Supported:             true,
		Text:                  "integration test",
	}
}

func tableCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, table string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM "+pgx.Identifier{table}.Sanitize()).Scan(&count); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return count
}

func countMessage(t *testing.T, ctx context.Context, pool *pgxpool.Pool, providerMessageID string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM whatsapp_messages WHERE provider_message_id = $1
	`, providerMessageID).Scan(&count); err != nil {
		t.Fatalf("count message: %v", err)
	}
	return count
}

func assertMessageState(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	providerMessageID string,
	wantStatus string,
	wantUserID *string,
) {
	t.Helper()
	var status string
	var userID *string
	if err := pool.QueryRow(ctx, `
		SELECT processing_status, resolved_user_id::text
		FROM whatsapp_messages
		WHERE provider_message_id = $1
	`, providerMessageID).Scan(&status, &userID); err != nil {
		t.Fatalf("load message state: %v", err)
	}
	if status != wantStatus || !equalOptionalString(userID, wantUserID) {
		t.Fatalf("message state = (%q, %v), want (%q, %v)", status, userID, wantStatus, wantUserID)
	}
}

func equalOptionalString(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func randomHex(t *testing.T, bytes int) string {
	t.Helper()
	buffer := make([]byte, bytes)
	if _, err := rand.Read(buffer); err != nil {
		t.Fatalf("generate schema suffix: %v", err)
	}
	return hex.EncodeToString(buffer)
}

type countingResolver struct {
	mu         sync.Mutex
	calls      int
	resolution identity.Resolution
}

func (r *countingResolver) ResolveIdentity(context.Context, identity.Provider, string) (identity.Resolution, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	return r.resolution, nil
}

func (r *countingResolver) ResolveIdentityWithScope(ctx context.Context, _ any, provider identity.Provider, providerUserID string) (identity.Resolution, error) {
	return r.ResolveIdentity(ctx, provider, providerUserID)
}

func (r *countingResolver) CallCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

type failOnceScopedResolver struct {
	mu       sync.Mutex
	calls    int
	delegate *identity.Resolver
}

func (r *failOnceScopedResolver) ResolveIdentity(ctx context.Context, provider identity.Provider, providerUserID string) (identity.Resolution, error) {
	if r.shouldFail() {
		return identity.Resolution{}, fmt.Errorf("temporary identity lookup failure")
	}
	return r.delegate.ResolveIdentity(ctx, provider, providerUserID)
}

func (r *failOnceScopedResolver) ResolveIdentityWithScope(ctx context.Context, scope any, provider identity.Provider, providerUserID string) (identity.Resolution, error) {
	if r.shouldFail() {
		return identity.Resolution{}, fmt.Errorf("temporary identity lookup failure")
	}
	return r.delegate.ResolveIdentityWithScope(ctx, scope, provider, providerUserID)
}

func (r *failOnceScopedResolver) shouldFail() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	return r.calls == 1
}

func (r *failOnceScopedResolver) CallCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}
