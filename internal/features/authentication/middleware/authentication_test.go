package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/harzeezco/zeexco_commerce/internal/features/authentication/models"
)

func TestAuthenticationMiddlewareCreatesPrincipal(t *testing.T) {
	verifier := &fakeVerifier{principal: models.Principal{UserID: testMiddlewareUserID}}
	handler := NewAuthentication(verifier).Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := PrincipalFromContext(r.Context())
		if !ok || principal.UserID != testMiddlewareUserID {
			t.Fatalf("principal=%#v ok=%v", principal, ok)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	request.Header.Set("Authorization", "Bearer valid-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || verifier.raw != "valid-token" {
		t.Fatalf("status=%d raw=%q", response.Code, verifier.raw)
	}
}

func TestAuthenticationMiddlewareRejectsMissingMalformedAndInvalidBearer(t *testing.T) {
	tests := []struct {
		name   string
		header []string
		err    error
	}{
		{name: "missing"},
		{name: "basic", header: []string{"Basic value"}},
		{name: "missing token", header: []string{"Bearer"}},
		{name: "extra parts", header: []string{"Bearer one two"}},
		{name: "duplicate headers", header: []string{"Bearer one", "Bearer two"}},
		{name: "invalid token", header: []string{"Bearer invalid"}, err: errors.New("invalid")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			called := false
			verifier := &fakeVerifier{principal: models.Principal{UserID: testMiddlewareUserID}, err: test.err}
			handler := NewAuthentication(verifier).Require(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
			request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
			for _, header := range test.header {
				request.Header.Add("Authorization", header)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusUnauthorized || called {
				t.Fatalf("status=%d called=%v", response.Code, called)
			}
		})
	}
}

const testMiddlewareUserID = "00000000-0000-4000-8000-000000000001"

type fakeVerifier struct {
	principal models.Principal
	err       error
	raw       string
}

func (v *fakeVerifier) Verify(raw string) (models.Principal, error) {
	v.raw = raw
	return v.principal, v.err
}
