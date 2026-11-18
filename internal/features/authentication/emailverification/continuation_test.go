package emailverification

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type continuationRepositoryStub struct {
	*verificationRepositoryStub
	continued bool
}

func (r *continuationRepositoryStub) VerifyWithContinuation(ctx context.Context, id string, now time.Time) (bool, error) {
	err := r.Verify(ctx, id, now)
	return r.continued, err
}

func TestVerifiedWhatsAppRegistrationReturnsConfiguredContinuation(t *testing.T) {
	tokens, _ := NewTokenManager([]byte("0123456789abcdef0123456789abcdef"))
	raw, _ := tokens.Mint("00000000-0000-4000-8000-000000000001", time.Now().Add(time.Hour))
	committed := false
	repository := &continuationRepositoryStub{verificationRepositoryStub: &verificationRepositoryStub{verify: func(context.Context, string, time.Time) error { committed = true; return nil }}, continued: true}
	service, err := NewService(repository, tokens, 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.SetWhatsAppReturnURL("https://wa.me/15551234567"); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	NewController(service).Verify(response, httptest.NewRequest(http.MethodPost, "/verify", strings.NewReader(`{"token":"`+raw+`"}`)))
	var payload struct {
		StatusCode  int    `json:"status_code"`
		NextStep    string `json:"next_step"`
		RedirectURL string `json:"redirect_url"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !committed || response.Code != 200 || payload.StatusCode != 200 || payload.NextStep != "CONTINUE_IN_WHATSAPP" || payload.RedirectURL != "https://wa.me/15551234567?text=Continue" {
		t.Fatalf("unexpected activation response: %s", response.Body.String())
	}
	if len(response.Result().Cookies()) != 0 || strings.Contains(response.Body.String(), raw) {
		t.Fatal("activation leaked credentials or established a browser session")
	}
	repository.continued = false
	response = httptest.NewRecorder()
	NewController(service).Verify(response, httptest.NewRequest(http.MethodPost, "/verify", strings.NewReader(`{"token":"`+raw+`"}`)))
	if response.Code != http.StatusNoContent {
		t.Fatal("ordinary web activation contract changed")
	}
}

func TestWhatsAppContinuationRejectsUntrustedDestinations(t *testing.T) {
	service := &Service{}
	for _, raw := range []string{"https://attacker.example/15551234567", "http://wa.me/15551234567", "https://wa.me/123/456", "https://user:password@wa.me/15551234567", "https://wa.me/01234567890", "https://wa.me/15551234567#fragment"} {
		if service.SetWhatsAppReturnURL(raw) == nil {
			t.Fatalf("accepted untrusted continuation destination %q", raw)
		}
	}
}
