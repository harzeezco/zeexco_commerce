package emailverification

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResendClientSendsIdempotentEmail(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/emails" || r.Header.Get("Authorization") != "Bearer re_test" ||
			r.Header.Get("Idempotency-Key") != "email-verification/job-1" {
			t.Fatalf("request path=%s headers=%v", r.URL.Path, r.Header)
		}
		var payload struct {
			To []string `json:"to"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || len(payload.To) != 1 || payload.To[0] != "ada@example.com" {
			t.Fatalf("payload=%#v error=%v", payload, err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"email_123"}`))
	}))
	defer server.Close()

	client, err := NewResendClient(ResendConfig{
		APIKey: "re_test", From: "Zeexco <hello@zeexco.test>",
		BaseURL: server.URL, HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	id, err := client.Send(context.Background(), "job-1", "ada@example.com", "Verify", "<p>Verify</p>", "Verify")
	if err != nil || id != "email_123" {
		t.Fatalf("id=%q error=%v", id, err)
	}
}
