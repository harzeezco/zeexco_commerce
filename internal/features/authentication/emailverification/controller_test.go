package emailverification

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type verifierFunc func(context.Context, string) error

func (f verifierFunc) Verify(ctx context.Context, token string) error { return f(ctx, token) }

func TestControllerVerifiesToken(t *testing.T) {
	controller := NewController(verifierFunc(func(_ context.Context, token string) error {
		if token != "signed-token" {
			t.Fatalf("token=%q", token)
		}
		return nil
	}))
	response := httptest.NewRecorder()
	controller.Verify(response, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"token":"signed-token"}`)))
	if response.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestControllerMapsExpiredToken(t *testing.T) {
	controller := NewController(verifierFunc(func(context.Context, string) error {
		return ErrVerificationExpired
	}))
	response := httptest.NewRecorder()
	controller.Verify(response, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"token":"signed-token"}`)))
	if response.Code != http.StatusGone || !strings.Contains(response.Body.String(), "EMAIL_VERIFICATION_EXPIRED") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestControllerRejectsInvalidJSON(t *testing.T) {
	controller := NewController(verifierFunc(func(context.Context, string) error {
		return errors.New("must not run")
	}))
	response := httptest.NewRecorder()
	controller.Verify(response, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"token":"x"} {}`)))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", response.Code)
	}
}
