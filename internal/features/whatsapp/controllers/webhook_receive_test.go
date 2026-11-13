package controllers

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/harzeezco/zeexco_commerce/internal/features/identity"
	"github.com/harzeezco/zeexco_commerce/internal/features/whatsapp/ingestion"
	"github.com/harzeezco/zeexco_commerce/internal/features/whatsapp/models"
	whatsapproutes "github.com/harzeezco/zeexco_commerce/internal/features/whatsapp/routes"
)

func TestWebhookControllerReceiveAcceptsSupportedObjectWithValidSignature(t *testing.T) {
	body := []byte(`{
        "object":"whatsapp_business_account",
        "entry":[{"changes":[{
            "field":"messages",
            "value":{
                "metadata":{"phone_number_id":"business-phone-id-1"},
                "messages":[{
                    "from":"sender-wa-id-1",
                    "id":"message-id-1",
                    "timestamp":"1700000000",
                    "type":"text",
                    "text":{"body":"Hello Zeexco"}
                }]
            }
        }]}]
    }`)
	response := performWebhookRequest(newTestWebhookController(), body, signTestPayload(body))

	assertResponse(t, response, http.StatusOK, "OK")
}

func TestWebhookControllerReceiveRejectsMalformedAuthenticatedMessage(t *testing.T) {
	ingester := &fakeMessageIngester{}
	body := []byte(`{
        "object":"whatsapp_business_account",
        "entry":[{"changes":[{
            "field":"messages",
            "value":{"messages":[{"from":"sender-wa-id-1","type":"text","text":{"body":"Hello"}}]}
        }]}]
    }`)
	response := performWebhookRequest(
		newTestWebhookControllerWithIngester(ingester),
		body,
		signTestPayload(body),
	)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusBadRequest)
	}
	if ingester.calls != 1 {
		t.Fatalf("ingestion calls = %d, want 1 durable event attempt", ingester.calls)
	}
}

func TestWebhookControllerReceiveAcknowledgesAuthenticatedStatusOnlyEvent(t *testing.T) {
	ingester := &fakeMessageIngester{}
	body := []byte(`{
        "object":"whatsapp_business_account",
        "entry":[{"changes":[{
            "field":"messages",
            "value":{"statuses":[{"id":"status-id-1","status":"delivered"}]}
        }]}]
    }`)
	response := performWebhookRequest(
		newTestWebhookControllerWithIngester(ingester),
		body,
		signTestPayload(body),
	)

	assertResponse(t, response, http.StatusOK, "OK")
	if ingester.calls != 1 {
		t.Fatalf("ingestion calls = %d, want 1 status event", ingester.calls)
	}
}

func TestWebhookControllerReceiveRejectsInvalidSignatureBeforeParsing(t *testing.T) {
	ingester := &fakeMessageIngester{}
	body := []byte(`{"object":"whatsapp_business_account","entry":[`)
	response := performWebhookRequest(
		newTestWebhookControllerWithIngester(ingester),
		body,
		"sha256="+strings.Repeat("0", sha256.Size*2),
	)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusForbidden)
	}
	if ingester.calls != 0 {
		t.Fatalf("ingestion calls = %d, want 0 before signature verification", ingester.calls)
	}
}

func TestWebhookControllerReceiveAcknowledgesIngestionOutcomes(t *testing.T) {
	body := []byte(`{
        "object":"whatsapp_business_account",
        "entry":[{"changes":[{
            "field":"messages",
            "value":{
                "metadata":{"phone_number_id":"business-phone-id-1"},
                "messages":[
                    {"from":"sender-1","id":"message-1","type":"text","text":{"body":"One"}},
                    {"from":"sender-2","id":"message-2","type":"text","text":{"body":"Two"}},
                    {"from":"sender-3","id":"message-3","type":"text","text":{"body":"Three"}}
                ]
            }
        }]}]
    }`)
	ingester := &fakeMessageIngester{results: []ingestion.Result{
		{
			Status: ingestion.StatusNew,
			Identity: identity.Resolution{
				Registered: true,
				UserID:     "user-id-1",
			},
		},
		{Status: ingestion.StatusDuplicate},
		{Status: ingestion.StatusNew, Identity: identity.Resolution{Registered: false}},
	}}

	response := performWebhookRequest(
		newTestWebhookControllerWithIngester(ingester),
		body,
		signTestPayload(body),
	)

	assertResponse(t, response, http.StatusOK, "OK")
	if ingester.calls != 1 || len(ingester.messages) != 3 {
		t.Fatalf("ingestion calls = %d, messages = %d", ingester.calls, len(ingester.messages))
	}
}

func TestWebhookControllerReceivePersistsBeforeAsynchronousProcessing(t *testing.T) {
	repository := &controllerMemoryMessageRepository{
		records: make(map[string]ingestion.Message),
	}
	resolver := &controllerIdentityResolver{
		resolutions: map[string]identity.Resolution{
			"registered-sender": {Registered: true, UserID: "user-id-1"},
		},
	}
	service, err := ingestion.NewService(
		repository,
		resolver,
		func() time.Time { return time.Unix(1800000000, 0).UTC() },
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	events := &controllerMemoryEventRepository{}
	service.SetWebhookEventRepository(events)

	body := []byte(`{
        "object":"whatsapp_business_account",
        "entry":[{"changes":[{
            "field":"messages",
            "value":{
                "metadata":{"phone_number_id":"business-phone-id-1"},
                "messages":[
                    {"from":"registered-sender","id":"message-1","type":"text","text":{"body":"One"}},
                    {"from":"unknown-sender","id":"message-2","type":"text","text":{"body":"Two"}}
                ]
            }
        }]}]
    }`)
	controller := newTestWebhookControllerWithIngester(service)

	firstResponse := performWebhookRequest(controller, body, signTestPayload(body))
	assertResponse(t, firstResponse, http.StatusOK, "OK")
	if events.next != 1 || len(repository.records) != 0 || resolver.resolveCalls != 0 || resolver.writeCalls != 0 {
		t.Fatalf(
			"events = %d records = %d, resolves = %d, writes = %d",
			events.next, len(repository.records),
			resolver.resolveCalls,
			resolver.writeCalls,
		)
	}
}

func TestWebhookControllerAcknowledgesBeforeDownstreamFailure(t *testing.T) {
	repository := &controllerMemoryMessageRepository{
		records: make(map[string]ingestion.Message),
	}
	resolver := &controllerIdentityResolver{
		resolutions: map[string]identity.Resolution{
			"registered-sender": {Registered: true, UserID: "user-id-1"},
		},
		failuresRemaining: 1,
	}
	service, err := ingestion.NewService(
		repository,
		resolver,
		func() time.Time { return time.Unix(1800000000, 0).UTC() },
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	service.SetWebhookEventRepository(&controllerMemoryEventRepository{})
	body := []byte(`{
        "object":"whatsapp_business_account",
        "entry":[{"changes":[{
            "field":"messages",
            "value":{
                "metadata":{"phone_number_id":"business-phone-id-1"},
                "messages":[{"from":"registered-sender","id":"resumable-message","type":"text","text":{"body":"One"}}]
            }
        }]}]
    }`)
	controller := newTestWebhookControllerWithIngester(service)

	firstResponse := performWebhookRequest(controller, body, signTestPayload(body))
	assertResponse(t, firstResponse, http.StatusOK, "OK")
	if len(repository.records) != 0 || resolver.resolveCalls != 0 {
		t.Fatalf("downstream processing ran before acknowledgement")
	}
}

func TestWebhookControllerReceiveReturnsServerErrorWhenIngestionFails(t *testing.T) {
	body := []byte(`{
        "object":"whatsapp_business_account",
        "entry":[{"changes":[{
            "field":"messages",
            "value":{
                "metadata":{"phone_number_id":"business-phone-id-1"},
                "messages":[{
                    "from":"sender-1","id":"message-1","type":"text","text":{"body":"Hello"}
                }]
            }
        }]}]
    }`)
	ingester := &fakeMessageIngester{err: errors.New("simulated database failure")}

	response := performWebhookRequest(
		newTestWebhookControllerWithIngester(ingester),
		body,
		signTestPayload(body),
	)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusInternalServerError)
	}
	if strings.Contains(response.Body.String(), "database") {
		t.Fatalf("response exposes database details: %q", response.Body.String())
	}
}

func TestWebhookControllerReceiveRejectsInvalidSignatures(t *testing.T) {
	body := []byte(`{"object":"whatsapp_business_account"}`)
	tests := []struct {
		name      string
		signature string
	}{
		{name: "invalid signature", signature: "sha256=" + strings.Repeat("0", sha256.Size*2)},
		{name: "missing signature", signature: ""},
		{name: "missing algorithm prefix", signature: strings.Repeat("0", sha256.Size*2)},
		{name: "wrong algorithm prefix", signature: "sha1=" + strings.Repeat("0", sha256.Size*2)},
		{name: "non hexadecimal signature", signature: "sha256=not-hexadecimal"},
		{name: "incorrect signature length", signature: "sha256=abcd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := performWebhookRequest(newTestWebhookController(), body, tt.signature)

			if response.Code != http.StatusForbidden {
				t.Fatalf("status code = %d, want %d", response.Code, http.StatusForbidden)
			}
		})
	}
}

func TestWebhookControllerReceiveRejectsPayloadTampering(t *testing.T) {
	originalBody := []byte(`{"object":"whatsapp_business_account"}`)
	tamperedBody := []byte(`{"object":"different_object"}`)

	response := performWebhookRequest(
		newTestWebhookController(),
		tamperedBody,
		signTestPayload(originalBody),
	)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusForbidden)
	}
}

func TestWebhookControllerReceiveRejectsDuplicateSignatureHeaders(t *testing.T) {
	body := []byte(`{"object":"whatsapp_business_account"}`)
	signature := signTestPayload(body)
	request := httptest.NewRequest(http.MethodPost, "/webhooks/whatsapp", bytes.NewReader(body))
	request.Header.Add(signatureHeader, signature)
	request.Header.Add(signatureHeader, signature)
	response := httptest.NewRecorder()

	newTestWebhookController().Receive(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusForbidden)
	}
}

func TestWebhookControllerReceiveRejectsInvalidJSONEnvelope(t *testing.T) {
	tests := []struct {
		name string
		body []byte
	}{
		{name: "malformed JSON", body: []byte(`{"object":`)},
		{name: "JSON array", body: []byte(`[]`)},
		{name: "missing object", body: []byte(`{"entry":[]}`)},
		{name: "null object", body: []byte(`{"object":null}`)},
		{name: "numeric object", body: []byte(`{"object":123}`)},
		{name: "empty object", body: []byte(`{"object":""}`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := performWebhookRequest(
				newTestWebhookController(),
				tt.body,
				signTestPayload(tt.body),
			)

			if response.Code != http.StatusBadRequest {
				t.Fatalf("status code = %d, want %d", response.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestWebhookControllerReceiveAcknowledgesUnsupportedObject(t *testing.T) {
	ingester := &fakeMessageIngester{}
	body := []byte(`{"object":"unsupported_object"}`)
	response := performWebhookRequest(
		newTestWebhookControllerWithIngester(ingester),
		body,
		signTestPayload(body),
	)

	assertResponse(t, response, http.StatusOK, "OK")
	if ingester.calls != 1 {
		t.Fatalf("ingestion calls = %d, want 1 unsupported event", ingester.calls)
	}
}

func TestWebhookControllerReceiveRejectsOversizedPayload(t *testing.T) {
	body := bytes.Repeat([]byte("a"), int(maxWebhookBodyBytes)+1)
	response := performWebhookRequest(newTestWebhookController(), body, signTestPayload(body))

	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestWebhookControllerReceiveLogsOnlySafeMetadata(t *testing.T) {
	const secretMarker = "secret-must-not-be-logged"
	const bodyMarker = "customer-message-must-not-be-logged"

	body := []byte(`{
        "object":"whatsapp_business_account",
        "entry":[{"changes":[{
            "field":"messages",
            "value":{
                "metadata":{"phone_number_id":"business-phone-id-1"},
                "messages":[{
                "from":"sender-wa-id-1",
                "id":"message-id-1",
                "timestamp":"1700000000",
                "type":"text",
                "text":{"body":"` + bodyMarker + `"}
            }]}
        }]}]
    }`)
	mac := hmac.New(sha256.New, []byte(secretMarker))
	_, _ = mac.Write(body)
	signature := signaturePrefix + hex.EncodeToString(mac.Sum(nil))

	var logs bytes.Buffer
	controller := NewWebhookController(
		testVerifyToken,
		secretMarker,
		&fakeMessageIngester{},
		log.New(&logs, "", 0),
	)
	response := performWebhookRequest(controller, body, signature)

	if response.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusOK)
	}

	logOutput := logs.String()
	if strings.Contains(logOutput, secretMarker) ||
		strings.Contains(logOutput, bodyMarker) ||
		strings.Contains(logOutput, "sender-wa-id-1") ||
		strings.Contains(logOutput, "message-id-1") ||
		strings.Contains(logOutput, signature) {
		t.Fatalf("log output contains sensitive request data: %q", logOutput)
	}

	if !strings.Contains(logOutput, "object=whatsapp_business_account outcome=durable event_id=test-event-id") {
		t.Fatalf("log output = %q, want safe accepted-event metadata", logOutput)
	}
}

func TestWebhookRoutesSupportGetVerificationAndPostDelivery(t *testing.T) {
	controller := newTestWebhookController()
	mux := http.NewServeMux()
	mux.HandleFunc(whatsapproutes.VerifyWebhook, controller.Verify)
	mux.HandleFunc(whatsapproutes.ReceiveWebhook, controller.Receive)

	getRequest := httptest.NewRequest(
		http.MethodGet,
		"/webhooks/whatsapp?hub.mode=subscribe&hub.verify_token="+testVerifyToken+"&hub.challenge=route-challenge",
		nil,
	)
	getResponse := httptest.NewRecorder()
	mux.ServeHTTP(getResponse, getRequest)
	assertResponse(t, getResponse, http.StatusOK, "route-challenge")

	body := []byte(`{"object":"whatsapp_business_account"}`)
	postRequest := httptest.NewRequest(http.MethodPost, "/webhooks/whatsapp", bytes.NewReader(body))
	postRequest.Header.Set(signatureHeader, signTestPayload(body))
	postResponse := httptest.NewRecorder()
	mux.ServeHTTP(postResponse, postRequest)
	assertResponse(t, postResponse, http.StatusOK, "OK")
}

func performWebhookRequest(
	controller *WebhookController,
	body []byte,
	signature string,
) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/webhooks/whatsapp", bytes.NewReader(body))
	if signature != "" {
		request.Header.Set(signatureHeader, signature)
	}

	response := httptest.NewRecorder()
	controller.Receive(response, request)
	return response
}

func signTestPayload(body []byte) string {
	mac := hmac.New(sha256.New, []byte(testMetaAppSecret))
	_, _ = mac.Write(body)
	return signaturePrefix + hex.EncodeToString(mac.Sum(nil))
}

func assertResponse(
	t *testing.T,
	response *httptest.ResponseRecorder,
	wantStatus int,
	wantBody string,
) {
	t.Helper()

	if response.Code != wantStatus {
		t.Fatalf("status code = %d, want %d", response.Code, wantStatus)
	}

	if got := response.Body.String(); got != wantBody {
		t.Fatalf("body = %q, want %q", got, wantBody)
	}
}

type controllerMemoryMessageRepository struct {
	records     map[string]ingestion.Message
	resolutions map[string]identity.Resolution
}

type controllerMemoryEventRepository struct{ next int }

func (r *controllerMemoryEventRepository) Store(_ context.Context, event models.WebhookEvent) (models.WebhookEvent, error) {
	r.next++
	event.ID = fmt.Sprintf("event-%d", r.next)
	return event, nil
}

func (*controllerMemoryEventRepository) ClaimNext(context.Context, time.Time, time.Time, time.Time, int) (models.WebhookEvent, bool, error) {
	return models.WebhookEvent{}, false, nil
}

func (*controllerMemoryEventRepository) MarkProcessing(context.Context, string, time.Time) error {
	return nil
}
func (*controllerMemoryEventRepository) MarkProcessed(context.Context, string, time.Time) error {
	return nil
}
func (*controllerMemoryEventRepository) MarkFailed(context.Context, string, time.Time, string, string) error {
	return nil
}

func (r *controllerMemoryMessageRepository) CompleteIdentityResolution(
	ctx context.Context,
	providerMessageID string,
	resolve func(context.Context, any) (identity.Resolution, error),
) (identity.Resolution, bool, error) {
	if r.resolutions == nil {
		r.resolutions = make(map[string]identity.Resolution)
	}
	message, exists := r.records[providerMessageID]
	if !exists {
		return identity.Resolution{}, false, errors.New("message not found")
	}
	if message.ProcessingStatus == ingestion.ProcessingStatusIdentityResolved {
		return r.resolutions[providerMessageID], false, nil
	}
	resolution, err := resolve(ctx, nil)
	if err != nil {
		return identity.Resolution{}, false, err
	}
	message.ProcessingStatus = ingestion.ProcessingStatusIdentityResolved
	r.records[providerMessageID] = message
	r.resolutions[providerMessageID] = resolution
	return resolution, true, nil
}

func (r *controllerMemoryMessageRepository) Store(
	_ context.Context,
	message ingestion.Message,
) (bool, error) {
	if _, exists := r.records[message.ProviderMessageID]; exists {
		return false, nil
	}
	r.records[message.ProviderMessageID] = message
	return true, nil
}

type controllerIdentityResolver struct {
	resolutions       map[string]identity.Resolution
	resolveCalls      int
	writeCalls        int
	failuresRemaining int
}

func (r *controllerIdentityResolver) ResolveIdentity(
	_ context.Context,
	provider identity.Provider,
	providerUserID string,
) (identity.Resolution, error) {
	if provider != identity.ProviderWhatsApp {
		return identity.Resolution{}, errors.New("unexpected provider")
	}
	r.resolveCalls++
	if r.failuresRemaining > 0 {
		r.failuresRemaining--
		return identity.Resolution{}, errors.New("temporary identity lookup failure")
	}
	return r.resolutions[providerUserID], nil
}

func (r *controllerIdentityResolver) ResolveIdentityWithScope(
	ctx context.Context,
	_ any,
	provider identity.Provider,
	providerUserID string,
) (identity.Resolution, error) {
	return r.ResolveIdentity(ctx, provider, providerUserID)
}

var _ WebhookIngester = (*ingestion.Service)(nil)
var _ ingestion.MessageRepository = (*controllerMemoryMessageRepository)(nil)
var _ ingestion.IdentityResolver = (*controllerIdentityResolver)(nil)
