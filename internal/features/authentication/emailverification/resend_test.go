package emailverification

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResendClientUsesPurposeSenderAndStableIdempotency(t *testing.T) {
	var fromValues, idempotencyValues []string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/emails" || r.Header.Get("Authorization") != "Bearer re_test" {
			t.Fatalf("request path=%s headers=%v", r.URL.Path, r.Header)
		}
		var payload struct {
			From string   `json:"from"`
			To   []string `json:"to"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || len(payload.To) != 1 || payload.To[0] != "ada@example.com" {
			t.Fatalf("payload=%#v error=%v", payload, err)
		}
		fromValues = append(fromValues, payload.From)
		idempotencyValues = append(idempotencyValues, r.Header.Get("Idempotency-Key"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"email_123"}`))
	}))
	defer server.Close()

	client, err := NewResendClient(ResendConfig{
		APIKey: "re_test", SecurityFrom: "Zeexco <no-reply@zeexco.com>",
		LifecycleFrom: "Zeexco <hello@zeexco.com>",
		BaseURL:       server.URL, HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	id, err := client.Send(context.Background(), "job-1", EmailPurposeSecurity, "ada@example.com", "Verify", "<p>Verify</p>", "Verify")
	if err != nil || id != "email_123" {
		t.Fatalf("id=%q error=%v", id, err)
	}
	_, err = client.Send(context.Background(), "job-2", EmailPurposeLifecycle, "ada@example.com", "Welcome", "<p>Welcome</p>", "Welcome")
	if err != nil {
		t.Fatal(err)
	}
	if fromValues[0] != "Zeexco <no-reply@zeexco.com>" || fromValues[1] != "Zeexco <hello@zeexco.com>" ||
		idempotencyValues[0] != "email-verification/job-1" || idempotencyValues[1] != "welcome/job-2" {
		t.Fatalf("from=%v idempotency=%v", fromValues, idempotencyValues)
	}
}

func TestResendClientRejectsMissingOrMalformedSenders(t *testing.T) {
	tests := []ResendConfig{
		{APIKey: "re_test", LifecycleFrom: "Zeexco <hello@zeexco.com>"},
		{APIKey: "re_test", SecurityFrom: "invalid", LifecycleFrom: "Zeexco <hello@zeexco.com>"},
		{APIKey: "re_test", SecurityFrom: "Zeexco <no-reply@zeexco.com>"},
	}
	for _, config := range tests {
		if _, err := NewResendClient(config); err == nil {
			t.Fatalf("accepted config %#v", config)
		}
	}
}
