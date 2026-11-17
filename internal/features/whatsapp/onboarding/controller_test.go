package onboarding

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	authmodels "github.com/harzeezco/zeexco_commerce/internal/features/authentication/models"
)

func TestValidateAPIReturnsOnlyPendingContext(t *testing.T) {
	raw, _, _ := NewToken()
	expiresAt := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	service := &fakeWebService{validateSession: Session{
		ID: "internal-session", ProviderUserID: "sensitive-wa-id",
		BusinessPhoneNumberID: "sensitive-phone-id", Status: StatusRegistrationPending,
		ExpiresAt: expiresAt,
	}}
	recorder := httptest.NewRecorder()
	NewController(service).ValidateAPI(recorder, httptest.NewRequest(
		http.MethodGet, "/api/v1/whatsapp/onboarding/validate?token="+url.QueryEscape(raw), nil,
	))
	if recorder.Code != http.StatusOK || recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("status=%d headers=%v", recorder.Code, recorder.Header())
	}
	var response validationResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.Valid || response.Status != "PENDING" || !response.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("response=%#v", response)
	}
	for _, sensitive := range []string{"internal-session", "sensitive-wa-id", "sensitive-phone-id", raw} {
		if strings.Contains(recorder.Body.String(), sensitive) {
			t.Fatalf("response exposed %q", sensitive)
		}
	}
}

func TestValidateAPIMapsStableTokenErrors(t *testing.T) {
	tests := []struct {
		name, target, code string
		err                error
		status             int
	}{
		{name: "missing", target: "/api/v1/whatsapp/onboarding/validate", status: 400, code: "ONBOARDING_TOKEN_REQUIRED"},
		{name: "duplicate parameter", target: "/api/v1/whatsapp/onboarding/validate?token=a&token=b", status: 400, code: "ONBOARDING_TOKEN_REQUIRED"},
		{name: "malformed", target: "/api/v1/whatsapp/onboarding/validate?token=bad", err: ErrMalformedToken, status: 400, code: "ONBOARDING_TOKEN_INVALID"},
		{name: "unknown", target: "/api/v1/whatsapp/onboarding/validate?token=unknown", err: ErrInvalidSession, status: 404, code: "ONBOARDING_TOKEN_INVALID"},
		{name: "expired", target: "/api/v1/whatsapp/onboarding/validate?token=expired", err: ErrSessionExpired, status: 410, code: "ONBOARDING_TOKEN_EXPIRED"},
		{name: "consumed", target: "/api/v1/whatsapp/onboarding/validate?token=consumed", err: ErrSessionConsumed, status: 409, code: "ONBOARDING_TOKEN_CONSUMED"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &fakeWebService{validateError: test.err}
			recorder := httptest.NewRecorder()
			NewController(service).ValidateAPI(recorder, httptest.NewRequest(http.MethodGet, test.target, nil))
			if recorder.Code != test.status || !strings.Contains(recorder.Body.String(), `"code":"`+test.code+`"`) {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestRegisterAPIIsExplicitTokenRequiredAndPasswordless(t *testing.T) {
	service := &fakeWebService{}
	controller := NewController(service)

	missing := httptest.NewRecorder()
	controller.RegisterAPI(missing, httptest.NewRequest(http.MethodPost,
		"/api/v1/whatsapp/onboarding/register", strings.NewReader(`{
			"first_name":"Ada","last_name":"Lovelace","business_name":"Commerce"
		}`)))
	if missing.Code != http.StatusBadRequest || !strings.Contains(missing.Body.String(), "ONBOARDING_TOKEN_REQUIRED") {
		t.Fatalf("missing token status=%d body=%s", missing.Code, missing.Body.String())
	}

	success := httptest.NewRecorder()
	controller.RegisterAPI(success, httptest.NewRequest(http.MethodPost,
		"/api/v1/whatsapp/onboarding/register", strings.NewReader(`{
			"first_name":"Ada","last_name":"Lovelace","business_name":"Commerce",
			"onboarding_token":"opaque-token"
		}`)))
	if success.Code != http.StatusCreated || service.completeToken != "opaque-token" || service.completeCommand.Email != "" {
		t.Fatalf("status=%d token=%q command=%#v body=%s", success.Code, service.completeToken, service.completeCommand, success.Body.String())
	}

	password := httptest.NewRecorder()
	controller.RegisterAPI(password, httptest.NewRequest(http.MethodPost,
		"/api/v1/whatsapp/onboarding/register", strings.NewReader(`{
			"first_name":"Ada","last_name":"Lovelace","business_name":"Commerce",
			"onboarding_token":"opaque-token","password":"must-not-be-accepted"
		}`)))
	if password.Code != http.StatusBadRequest || !strings.Contains(password.Body.String(), "INVALID_JSON") {
		t.Fatalf("password status=%d body=%s", password.Code, password.Body.String())
	}
}

func TestRegistrationPageAndSafeErrors(t *testing.T) {
	raw, _, _ := NewToken()
	service := &fakeWebService{returnURL: "https://wa.me/"}
	controller := NewController(service)

	recorder := httptest.NewRecorder()
	controller.Page(recorder, httptest.NewRequest(http.MethodGet, "/get-started?t="+url.QueryEscape(raw), nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `name="business_name"`) ||
		strings.Contains(recorder.Body.String(), `name="phone"`) || strings.Contains(recorder.Body.String(), `name="password"`) {
		t.Fatalf("unexpected registration page status=%d", recorder.Code)
	}
	if recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("sensitive page may be cached")
	}

	service.completeError = authmodels.ErrEmailAlreadyRegistered
	form := url.Values{"token": {raw}, "email": {"existing@example.com"}, "password": {"never-redisplay"}}
	recorder = httptest.NewRecorder()
	controller.Submit(recorder, httptest.NewRequest(http.MethodPost, "/get-started", strings.NewReader(form.Encode())))
	if recorder.Code != http.StatusConflict || strings.Contains(recorder.Body.String(), "never-redisplay") || !strings.Contains(recorder.Body.String(), "future authenticated linking flow") {
		t.Fatal("duplicate email was not handled safely")
	}
}

func TestConsumedPageShowsSuccessAndRetriesDelivery(t *testing.T) {
	userID := "user-id"
	service := &fakeWebService{validateSession: Session{LinkedUserID: &userID}, validateError: ErrSessionConsumed, returnURL: "https://wa.me/"}
	recorder := httptest.NewRecorder()
	NewController(service).Page(recorder, httptest.NewRequest(http.MethodGet, "/get-started?t=token", nil))
	if recorder.Code != http.StatusOK || service.retried != userID || !strings.Contains(recorder.Body.String(), "Continue in WhatsApp") {
		t.Fatal("consumed URL did not provide idempotent success state")
	}
}

type fakeWebService struct {
	validateSession              Session
	validateError, completeError error
	retried, returnURL           string
	completeToken                string
	completeCommand              RegisterCommand
}

func (f *fakeWebService) Validate(context.Context, string) (Session, error) {
	return f.validateSession, f.validateError
}
func (f *fakeWebService) Complete(_ context.Context, token string, command RegisterCommand) (Completion, error) {
	f.completeToken = token
	f.completeCommand = command
	return Completion{}, f.completeError
}
func (f *fakeWebService) RetryCompletion(_ context.Context, userID string) { f.retried = userID }
func (f *fakeWebService) ReturnURL() string                                { return f.returnURL }
