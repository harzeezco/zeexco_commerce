package cloudapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const (
	testAccessToken   = "test-only-access-token"
	testPhoneNumberID = "123456789"
	testAPIVersion    = "v99.0"
)

func TestSendTextMessageSendsExpectedRequestAndParsesResponse(t *testing.T) {
	type capturedRequest struct {
		method        string
		path          string
		authorization string
		contentType   string
		payload       sendTextRequest
		decodeError   error
	}

	captured := make(chan capturedRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request := capturedRequest{
			method:        r.Method,
			path:          r.URL.Path,
			authorization: r.Header.Get("Authorization"),
			contentType:   r.Header.Get("Content-Type"),
		}
		request.decodeError = json.NewDecoder(r.Body).Decode(&request.payload)
		captured <- request

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"messages":[{"id":"meta-message-id-1"}]}`))
	}))
	defer server.Close()

	var logs bytes.Buffer
	client := newTestClient(t, server.URL, server.Client(), log.New(&logs, "", 0))
	result, err := client.SendTextMessage(
		context.Background(),
		"recipient-wa-id-marker",
		"customer-message-marker",
	)
	if err != nil {
		t.Fatalf("SendTextMessage() error = %v", err)
	}

	if result.MessageID != "meta-message-id-1" {
		t.Fatalf("MessageID = %q, want %q", result.MessageID, "meta-message-id-1")
	}

	request := <-captured
	if request.decodeError != nil {
		t.Fatalf("decode request: %v", request.decodeError)
	}
	if request.method != http.MethodPost {
		t.Errorf("method = %q, want POST", request.method)
	}
	if request.path != "/v99.0/123456789/messages" {
		t.Errorf("path = %q, want configured messages endpoint", request.path)
	}
	if request.authorization != "Bearer "+testAccessToken {
		t.Errorf("Authorization header was not the configured bearer token")
	}
	if request.contentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", request.contentType)
	}

	wantPayload := sendTextRequest{
		MessagingProduct: "whatsapp",
		RecipientType:    "individual",
		To:               "recipient-wa-id-marker",
		Type:             "text",
		Text:             sendTextContent{Body: "customer-message-marker"},
	}
	if request.payload != wantPayload {
		t.Errorf("payload = %#v, want %#v", request.payload, wantPayload)
	}

	logOutput := logs.String()
	for _, sensitiveValue := range []string{
		testAccessToken,
		"recipient-wa-id-marker",
		"customer-message-marker",
	} {
		if strings.Contains(logOutput, sensitiveValue) {
			t.Fatalf("logs contain sensitive value %q: %q", sensitiveValue, logOutput)
		}
	}
	if !strings.Contains(logOutput, "operation=send_text outcome=success http_status=200") {
		t.Fatalf("logs = %q, want safe success metadata", logOutput)
	}
}

func TestInteractiveOnboardingPayloadsUseSupportedMetaShapes(t *testing.T) {
	tests := []struct {
		name   string
		send   func(*Client) error
		assert func(*testing.T, sendInteractiveRequest)
	}{
		{
			name: "external HTTPS CTA URL",
			send: func(client *Client) error {
				return client.SendCTAURLMessage(context.Background(), "recipient", "Welcome", "Get started", "https://commerce.example/get-started?t=opaque")
			},
			assert: func(t *testing.T, payload sendInteractiveRequest) {
				if payload.Interactive.Type != "cta_url" || payload.Interactive.Action.Name != "cta_url" ||
					payload.Interactive.Action.Parameters == nil || payload.Interactive.Action.Parameters.URL != "https://commerce.example/get-started?t=opaque" {
					t.Fatalf("CTA payload = %#v", payload)
				}
			},
		},
		{
			name: "merchant registration Flow",
			send: func(client *Client) error {
				return client.SendFlowMessage(
					context.Background(), "recipient", "Welcome", "Get started",
					"987654321", "opaque-onboarding-token",
				)
			},
			assert: func(t *testing.T, payload sendInteractiveRequest) {
				parameters := payload.Interactive.Action.Parameters
				if payload.Interactive.Type != "flow" || payload.Interactive.Action.Name != "flow" ||
					parameters == nil || parameters.FlowMessageVersion != "3" ||
					parameters.Mode != FlowModePublished ||
					parameters.FlowToken != "opaque-onboarding-token" || parameters.FlowID != "987654321" ||
					parameters.FlowCTA != "Get started" || parameters.FlowAction != "navigate" ||
					parameters.FlowActionPayload == nil || parameters.FlowActionPayload.Screen != "REGISTRATION" ||
					parameters.FlowActionPayload.Data["has_error"] != false {
					t.Fatalf("Flow payload = %#v", payload)
				}
			},
		},
		{
			name: "first product reply button",
			send: func(client *Client) error {
				return client.SendReplyButtonMessage(context.Background(), "recipient", "All set", "CREATE_FIRST_PRODUCT", "Create first product")
			},
			assert: func(t *testing.T, payload sendInteractiveRequest) {
				buttons := payload.Interactive.Action.Buttons
				if payload.Interactive.Type != "button" || len(buttons) != 1 || buttons[0].Type != "reply" || buttons[0].Reply.ID != "CREATE_FIRST_PRODUCT" {
					t.Fatalf("reply payload = %#v", payload)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var payload sendInteractiveRequest
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v99.0/123456789/messages" {
					t.Errorf("path = %q", r.URL.Path)
				}
				if r.Header.Get("Authorization") != "Bearer "+testAccessToken {
					t.Error("configured Bearer credential was not attached")
				}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Fatal(err)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"messages":[{"id":"meta-id"}]}`))
			}))
			defer server.Close()
			if err := test.send(newTestClient(t, server.URL, server.Client(), discardLogger())); err != nil {
				t.Fatal(err)
			}
			if payload.Type != "interactive" || payload.MessagingProduct != "whatsapp" {
				t.Fatalf("payload = %#v", payload)
			}
			test.assert(t, payload)
		})
	}
}

func TestDraftFlowMessagePreservesRegistrationPayload(t *testing.T) {
	var payload sendInteractiveRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(`{"messages":[{"id":"meta-id"}]}`))
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		AccessToken: testAccessToken, PhoneNumberID: testPhoneNumberID,
		APIVersion: testAPIVersion, FlowMode: FlowModeDraft,
		BaseURL: server.URL, HTTPClient: server.Client(), Logger: discardLogger(),
	})
	if err != nil {
		t.Fatal(err)
	}
	const token = "genuine-opaque-onboarding-token"
	if err := client.SendFlowMessage(
		context.Background(), "recipient", "Welcome", "Get started", "2198301500826080", token,
	); err != nil {
		t.Fatal(err)
	}

	parameters := payload.Interactive.Action.Parameters
	if parameters == nil || parameters.Mode != FlowModeDraft ||
		parameters.FlowMessageVersion != "3" || parameters.FlowToken != token ||
		parameters.FlowID != "2198301500826080" || parameters.FlowCTA != "Get started" ||
		parameters.FlowAction != "navigate" || parameters.FlowActionPayload == nil ||
		parameters.FlowActionPayload.Screen != "REGISTRATION" {
		t.Fatalf("draft Flow payload = %#v", payload)
	}
}

func TestNewClientRejectsUnsupportedFlowMode(t *testing.T) {
	_, err := NewClient(ClientConfig{
		AccessToken: testAccessToken, PhoneNumberID: testPhoneNumberID,
		APIVersion: testAPIVersion, FlowMode: "preview",
	})
	if err == nil || !strings.Contains(err.Error(), "WHATSAPP_FLOW_MODE") {
		t.Fatalf("NewClient() error=%v, want WHATSAPP_FLOW_MODE validation error", err)
	}
}

func TestFlowMessageDoesNotLeakTokenIntoLogsOrVisibleText(t *testing.T) {
	var raw map[string]any
	var logs bytes.Buffer
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(`{"messages":[{"id":"meta-id"}]}`))
	}))
	defer server.Close()
	client := newTestClient(t, server.URL, server.Client(), log.New(&logs, "", 0))
	token := "opaque-onboarding-token-marker"
	if err := client.SendFlowMessage(
		context.Background(), "recipient", "Welcome to Zeexco", "Get started",
		"987654321", token,
	); err != nil {
		t.Fatal(err)
	}
	interactive := raw["interactive"].(map[string]any)
	body := interactive["body"].(map[string]any)
	if body["text"] != "Welcome to Zeexco" || strings.Contains(body["text"].(string), token) || strings.Contains(logs.String(), token) {
		t.Fatalf("token leaked into visible text or logs: body=%#v logs=%q", body, logs.String())
	}
}

func TestFlowMessageRejectsInvalidConfigurationBeforeHTTP(t *testing.T) {
	doer := &countingDoer{}
	client := newTestClient(t, "https://graph.example.test", doer, discardLogger())
	for _, test := range []struct {
		flowID string
		token  string
	}{
		{flowID: "not-numeric", token: "token"},
		{flowID: "123", token: ""},
	} {
		if err := client.SendFlowMessage(context.Background(), "recipient", "Welcome", "Get started", test.flowID, test.token); !errors.Is(err, ErrFlowConfiguration) {
			t.Fatalf("flowID=%q token=%q error=%v", test.flowID, test.token, err)
		}
	}
	if doer.calls != 0 {
		t.Fatalf("HTTP calls=%d", doer.calls)
	}
}

func TestCTAURLProductionBuilderSerializesMetaSchema(t *testing.T) {
	var raw map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(`{"messages":[{"id":"meta-id"}]}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, server.Client(), log.New(io.Discard, "", 0))
	if err := client.SendCTAURLMessage(
		context.Background(), "recipient", "Welcome to Zeexco", "Get started",
		"https://commerce.example/get-started?t=opaque",
	); err != nil {
		t.Fatal(err)
	}
	interactive := raw["interactive"].(map[string]any)
	body := interactive["body"].(map[string]any)
	action := interactive["action"].(map[string]any)
	parameters := action["parameters"].(map[string]any)
	if raw["messaging_product"] != "whatsapp" || raw["recipient_type"] != "individual" ||
		raw["type"] != "interactive" || interactive["type"] != "cta_url" ||
		body["text"] != "Welcome to Zeexco" || body["body"] != nil ||
		action["name"] != "cta_url" || parameters["display_text"] != "Get started" ||
		parameters["url"] != "https://commerce.example/get-started?t=opaque" {
		t.Fatalf("serialized CTA payload = %#v", raw)
	}
}

func TestProviderStructuredErrorPreservesSafeDiagnosticsAndRedactsRequestData(t *testing.T) {
	var logs bytes.Buffer
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{
			"error": {
				"message": "Invalid parameter for recipient-marker",
				"type": "OAuthException",
				"code": 100,
				"error_subcode": 2494013,
				"error_data": {
					"messaging_product": "whatsapp",
					"details": "CTA URL https://commerce.example/get-started?t=opaque is invalid"
				},
				"fbtrace_id": "SafeTrace_123"
			}
		}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, server.Client(), log.New(&logs, "", 0))
	err := client.SendCTAURLMessage(
		context.Background(), "recipient-marker", "Welcome marker", "Get started",
		"https://commerce.example/get-started?t=opaque",
	)
	var providerError *ProviderError
	if !errors.As(err, &providerError) {
		t.Fatalf("error = %v, want ProviderError", err)
	}
	if providerError.Code != 100 || providerError.Subcode != 2494013 ||
		providerError.Type != "OAuthException" || providerError.MessagingProduct != "whatsapp" ||
		providerError.TraceID != "SafeTrace_123" {
		t.Fatalf("ProviderError = %#v", providerError)
	}
	if providerError.Message != "Invalid parameter for [redacted]" ||
		providerError.Details != "CTA URL [redacted] is invalid" {
		t.Fatalf("sanitized diagnostics = %#v", providerError)
	}
	combined := err.Error() + logs.String()
	for _, secret := range []string{testAccessToken, "recipient-marker", "Welcome marker", "https://commerce.example/get-started?t=opaque"} {
		if strings.Contains(combined, secret) {
			t.Fatalf("diagnostics leaked %q: %q", secret, combined)
		}
	}
	for _, useful := range []string{"provider_subcode=2494013", `provider_message="Invalid parameter for [redacted]"`, `provider_details="CTA URL [redacted] is invalid"`, `provider_trace_id="SafeTrace_123"`} {
		if !strings.Contains(logs.String(), useful) {
			t.Fatalf("logs %q missing %q", logs.String(), useful)
		}
	}
}

func TestCTARejectsNonHTTPSURLBeforeSending(t *testing.T) {
	doer := &countingDoer{}
	client := newTestClient(t, "https://graph.example.test", doer, discardLogger())
	if err := client.SendCTAURLMessage(context.Background(), "recipient", "Welcome", "Get started", "http://commerce.example/get-started"); err == nil {
		t.Fatal("non-HTTPS CTA unexpectedly accepted")
	}
	if doer.calls != 0 {
		t.Fatalf("HTTP calls = %d", doer.calls)
	}
}

func TestSendTextMessageRejectsInvalidInputBeforeHTTP(t *testing.T) {
	doer := &countingDoer{}
	client := newTestClient(t, "https://graph.example.test", doer, discardLogger())

	tests := []struct {
		name    string
		to      string
		body    string
		wantErr error
	}{
		{name: "empty recipient", to: "", body: "Hello", wantErr: ErrRecipientRequired},
		{name: "blank recipient", to: "   ", body: "Hello", wantErr: ErrRecipientRequired},
		{name: "empty message", to: "recipient", body: "", wantErr: ErrMessageBodyRequired},
		{name: "blank message", to: "recipient", body: "  ", wantErr: ErrMessageBodyRequired},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.SendTextMessage(context.Background(), tt.to, tt.body)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}

	if doer.calls != 0 {
		t.Fatalf("HTTP calls = %d, want 0", doer.calls)
	}
}

func TestSendTextMessageReturnsSanitizedProviderErrors(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		code       int
		errorType  string
	}{
		{name: "bad request", statusCode: http.StatusBadRequest, code: 100, errorType: "GraphMethodException"},
		{name: "unauthorized", statusCode: http.StatusUnauthorized, code: 190, errorType: "OAuthException"},
		{name: "forbidden", statusCode: http.StatusForbidden, code: 200, errorType: "OAuthException"},
		{name: "rate limited", statusCode: http.StatusTooManyRequests, code: 4, errorType: "ThrottlingException"},
		{name: "provider failure", statusCode: http.StatusInternalServerError, code: 2, errorType: "ServiceException"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.statusCode)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error": map[string]any{
						"message": "provider-message customer-message-marker recipient-marker " + testAccessToken,
						"type":    tt.errorType,
						"code":    tt.code,
					},
				})
			}))
			defer server.Close()

			client := newTestClient(t, server.URL, server.Client(), discardLogger())
			_, err := client.SendTextMessage(
				context.Background(),
				"recipient-marker",
				"customer-message-marker",
			)

			var providerError *ProviderError
			if !errors.As(err, &providerError) {
				t.Fatalf("error = %v, want ProviderError", err)
			}
			if providerError.StatusCode != tt.statusCode ||
				providerError.Code != tt.code ||
				providerError.Type != tt.errorType {
				t.Fatalf("ProviderError = %#v", providerError)
			}

			for _, sensitiveValue := range []string{
				testAccessToken,
				"recipient-marker",
				"customer-message-marker",
			} {
				if strings.Contains(err.Error(), sensitiveValue) {
					t.Fatalf("error leaks %q: %q", sensitiveValue, err.Error())
				}
			}
		})
	}
}

func TestSendTextMessageHandlesNetworkFailure(t *testing.T) {
	transportError := errors.New("simulated transport failure")
	client := newTestClient(
		t,
		"https://graph.example.test",
		doerFunc(func(*http.Request) (*http.Response, error) {
			return nil, transportError
		}),
		discardLogger(),
	)

	_, err := client.SendTextMessage(context.Background(), "recipient", "Hello")
	var networkError *NetworkError
	if !errors.As(err, &networkError) {
		t.Fatalf("error = %v, want NetworkError", err)
	}
	if !errors.Is(err, transportError) {
		t.Fatalf("error does not preserve transport cause: %v", err)
	}
}

func TestSendTextMessageRejectsMalformedSuccessResponse(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "invalid JSON", body: `{"messages":`},
		{name: "missing messages", body: `{}`},
		{name: "empty messages", body: `{"messages":[]}`},
		{name: "missing message ID", body: `{"messages":[{}]}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			client := newTestClient(t, server.URL, server.Client(), discardLogger())
			_, err := client.SendTextMessage(context.Background(), "recipient", "Hello")
			if !errors.Is(err, ErrMalformedResponse) {
				t.Fatalf("error = %v, want ErrMalformedResponse", err)
			}
		})
	}
}

func TestSendTextMessageHandlesRequestCreationFailure(t *testing.T) {
	doer := &countingDoer{}
	client := newTestClient(t, "https://graph.example.test", doer, discardLogger())

	_, err := client.SendTextMessage(nil, "recipient", "Hello")
	if err == nil || !strings.Contains(err.Error(), "create Meta WhatsApp request") {
		t.Fatalf("error = %v, want request creation error", err)
	}
	if doer.calls != 0 {
		t.Fatalf("HTTP calls = %d, want 0", doer.calls)
	}
}

func TestNewClientValidatesOutboundConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		config ClientConfig
	}{
		{
			name: "missing access token",
			config: ClientConfig{
				PhoneNumberID: testPhoneNumberID,
				APIVersion:    testAPIVersion,
			},
		},
		{
			name: "missing phone number ID",
			config: ClientConfig{
				AccessToken: testAccessToken,
				APIVersion:  testAPIVersion,
			},
		},
		{
			name: "missing API version",
			config: ClientConfig{
				AccessToken:   testAccessToken,
				PhoneNumberID: testPhoneNumberID,
			},
		},
		{
			name: "invalid phone number ID format",
			config: ClientConfig{
				AccessToken: testAccessToken, PhoneNumberID: "not-an-id", APIVersion: testAPIVersion,
			},
		},
		{
			name: "invalid API version format",
			config: ClientConfig{
				AccessToken: testAccessToken, PhoneNumberID: testPhoneNumberID, APIVersion: "latest",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewClient(tt.config); err == nil {
				t.Fatal("NewClient() error = nil, want configuration error")
			}
		})
	}
}

func TestNewClientUsesReasonableDefaultHTTPTimeout(t *testing.T) {
	client, err := NewClient(ClientConfig{
		AccessToken:   testAccessToken,
		PhoneNumberID: testPhoneNumberID,
		APIVersion:    testAPIVersion,
		Logger:        discardLogger(),
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	httpClient, ok := client.httpClient.(*http.Client)
	if !ok {
		t.Fatalf("HTTP client type = %T, want *http.Client", client.httpClient)
	}
	if httpClient.Timeout != 10*time.Second {
		t.Fatalf("HTTP timeout = %v, want %v", httpClient.Timeout, 10*time.Second)
	}
}

func TestDefaultClientTreatsRedirectAsProviderFailure(t *testing.T) {
	targetCalls := 0
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		targetCalls++
	}))
	defer target.Close()

	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", target.URL)
		w.WriteHeader(http.StatusFound)
	}))
	defer redirect.Close()

	client, err := NewClient(ClientConfig{
		AccessToken:   testAccessToken,
		PhoneNumberID: testPhoneNumberID,
		APIVersion:    testAPIVersion,
		BaseURL:       redirect.URL,
		Logger:        discardLogger(),
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	_, err = client.SendTextMessage(context.Background(), "recipient", "Hello")
	var providerError *ProviderError
	if !errors.As(err, &providerError) || providerError.StatusCode != http.StatusFound {
		t.Fatalf("error = %v, want HTTP 302 ProviderError", err)
	}
	if targetCalls != 0 {
		t.Fatalf("redirect target calls = %d, want 0", targetCalls)
	}
}

func TestSendTextMessageClosesResponseBody(t *testing.T) {
	body := &trackingReadCloser{
		Reader: strings.NewReader(`{"messages":[{"id":"meta-message-id-1"}]}`),
	}
	client := newTestClient(
		t,
		"https://graph.example.test",
		doerFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       body,
				Header:     make(http.Header),
			}, nil
		}),
		discardLogger(),
	)

	if _, err := client.SendTextMessage(context.Background(), "recipient", "Hello"); err != nil {
		t.Fatalf("SendTextMessage() error = %v", err)
	}
	if !body.closed {
		t.Fatal("response body was not closed")
	}
}

func newTestClient(
	t *testing.T,
	baseURL string,
	httpClient HTTPDoer,
	logger *log.Logger,
) *Client {
	t.Helper()

	client, err := NewClient(ClientConfig{
		AccessToken:   testAccessToken,
		PhoneNumberID: testPhoneNumberID,
		APIVersion:    testAPIVersion,
		BaseURL:       baseURL,
		HTTPClient:    httpClient,
		Logger:        logger,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	return client
}

func discardLogger() *log.Logger {
	return log.New(io.Discard, "", 0)
}

type countingDoer struct {
	calls int
}

func (d *countingDoer) Do(*http.Request) (*http.Response, error) {
	d.calls++
	return nil, errors.New("unexpected HTTP request")
}

type doerFunc func(*http.Request) (*http.Response, error)

func (f doerFunc) Do(request *http.Request) (*http.Response, error) {
	return f(request)
}

type trackingReadCloser struct {
	io.Reader
	closed bool
}

func (b *trackingReadCloser) Close() error {
	b.closed = true
	return nil
}
