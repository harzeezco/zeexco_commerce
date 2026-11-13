package controllers

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/harzeezco/zeexco_commerce/internal/features/whatsapp/ingestion"
	"github.com/harzeezco/zeexco_commerce/internal/features/whatsapp/models"
	whatsappparser "github.com/harzeezco/zeexco_commerce/internal/features/whatsapp/parser"
)

const testVerifyToken = "test-only-verification-token"
const testMetaAppSecret = "test-only-meta-app-secret"

func newTestWebhookController() *WebhookController {
	return newTestWebhookControllerWithIngester(&fakeMessageIngester{})
}

func newTestWebhookControllerWithIngester(ingester WebhookIngester) *WebhookController {
	return NewWebhookController(
		testVerifyToken,
		testMetaAppSecret,
		ingester,
		log.New(io.Discard, "", 0),
	)
}

type fakeMessageIngester struct {
	results  []ingestion.Result
	err      error
	calls    int
	messages []models.IncomingMessage
}

func (f *fakeMessageIngester) AcceptWebhook(_ context.Context, payload []byte) (ingestion.WebhookResult, error) {
	f.calls++
	if f.err != nil {
		return ingestion.WebhookResult{}, f.err
	}
	var envelope models.WebhookEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return ingestion.WebhookResult{}, &ingestion.InvalidWebhookError{Err: err}
	}
	result := ingestion.WebhookResult{EventID: "test-event-id", Object: envelope.Object}
	if envelope.Object != whatsappparser.SupportedWebhookObject {
		return result, nil
	}
	messages, err := whatsappparser.ParseMessages(payload)
	if err != nil {
		return result, &ingestion.InvalidWebhookError{Err: err}
	}
	f.messages = append([]models.IncomingMessage(nil), messages...)
	result.Messages = messages
	if f.results != nil {
		result.Results = append([]ingestion.Result(nil), f.results...)
		return result, nil
	}

	results := make([]ingestion.Result, len(messages))
	for index := range results {
		results[index] = ingestion.Result{Status: ingestion.StatusNew}
	}
	result.Results = results
	return result, nil
}

func TestWebhookControllerVerify(t *testing.T) {
	tests := []struct {
		name        string
		verifyToken string
		target      string
		wantStatus  int
	}{
		{
			name:        "valid mode token and challenge",
			verifyToken: testVerifyToken,
			target:      "/webhooks/whatsapp?hub.mode=subscribe&hub.verify_token=test-only-verification-token&hub.challenge=challenge-123",
			wantStatus:  http.StatusOK,
		},
		{
			name:        "incorrect token",
			verifyToken: testVerifyToken,
			target:      "/webhooks/whatsapp?hub.mode=subscribe&hub.verify_token=wrong-token&hub.challenge=challenge-123",
			wantStatus:  http.StatusForbidden,
		},
		{
			name:        "incorrect mode",
			verifyToken: testVerifyToken,
			target:      "/webhooks/whatsapp?hub.mode=unsubscribe&hub.verify_token=test-only-verification-token&hub.challenge=challenge-123",
			wantStatus:  http.StatusForbidden,
		},
		{
			name:        "missing mode",
			verifyToken: testVerifyToken,
			target:      "/webhooks/whatsapp?hub.verify_token=test-only-verification-token&hub.challenge=challenge-123",
			wantStatus:  http.StatusForbidden,
		},
		{
			name:        "missing token",
			verifyToken: testVerifyToken,
			target:      "/webhooks/whatsapp?hub.mode=subscribe&hub.challenge=challenge-123",
			wantStatus:  http.StatusForbidden,
		},
		{
			name:        "empty token",
			verifyToken: testVerifyToken,
			target:      "/webhooks/whatsapp?hub.mode=subscribe&hub.verify_token=&hub.challenge=challenge-123",
			wantStatus:  http.StatusForbidden,
		},
		{
			name:        "missing challenge",
			verifyToken: testVerifyToken,
			target:      "/webhooks/whatsapp?hub.mode=subscribe&hub.verify_token=test-only-verification-token",
			wantStatus:  http.StatusForbidden,
		},
		{
			name:        "empty challenge",
			verifyToken: testVerifyToken,
			target:      "/webhooks/whatsapp?hub.mode=subscribe&hub.verify_token=test-only-verification-token&hub.challenge=",
			wantStatus:  http.StatusForbidden,
		},
		{
			name:        "missing configuration",
			verifyToken: "",
			target:      "/webhooks/whatsapp?hub.mode=subscribe&hub.verify_token=test-only-verification-token&hub.challenge=challenge-123",
			wantStatus:  http.StatusForbidden,
		},
		{
			name:        "duplicate token parameter",
			verifyToken: testVerifyToken,
			target:      "/webhooks/whatsapp?hub.mode=subscribe&hub.verify_token=test-only-verification-token&hub.verify_token=wrong-token&hub.challenge=challenge-123",
			wantStatus:  http.StatusForbidden,
		},
		{
			name:        "duplicate challenge parameter",
			verifyToken: testVerifyToken,
			target:      "/webhooks/whatsapp?hub.mode=subscribe&hub.verify_token=test-only-verification-token&hub.challenge=challenge-123&hub.challenge=another-challenge",
			wantStatus:  http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controller := NewWebhookController(
				tt.verifyToken,
				testMetaAppSecret,
				&fakeMessageIngester{},
				log.New(io.Discard, "", 0),
			)
			request := httptest.NewRequest(http.MethodGet, tt.target, nil)
			response := httptest.NewRecorder()

			controller.Verify(response, request)

			if response.Code != tt.wantStatus {
				t.Fatalf("status code = %d, want %d", response.Code, tt.wantStatus)
			}
		})
	}
}

func TestWebhookControllerVerifyReturnsExactChallengeAsPlainText(t *testing.T) {
	controller := newTestWebhookController()
	request := httptest.NewRequest(
		http.MethodGet,
		"/webhooks/whatsapp?hub.mode=subscribe&hub.verify_token=test-only-verification-token&hub.challenge=exact%2Bchallenge%3Dvalue",
		nil,
	)
	response := httptest.NewRecorder()

	controller.Verify(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusOK)
	}

	if got := response.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type = %q, want %q", got, "text/plain; charset=utf-8")
	}

	if got := response.Body.String(); got != "exact+challenge=value" {
		t.Errorf("body = %q, want %q", got, "exact+challenge=value")
	}
}
