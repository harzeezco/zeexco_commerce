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

func TestCookieAuthenticationAndCSRF(t *testing.T) {
	for _, test := range []struct {
		name, method, origin string
		cookies              []string
		verifyErr            error
		wantStatus           int
	}{
		{"read with cookie", "GET", "", []string{"valid-token"}, nil, 204},
		{"write trusted origin", "POST", "https://app.example", []string{"valid-token"}, nil, 204},
		{"write missing origin", "POST", "", []string{"valid-token"}, nil, 403},
		{"write hostile origin", "POST", "https://attacker.example", []string{"valid-token"}, nil, 403},
		{"write similar origin", "POST", "https://app.example.attacker.example", []string{"valid-token"}, nil, 403},
		{"empty cookie", "GET", "", []string{""}, nil, 401},
		{"invalid or expired cookie", "GET", "", []string{"expired"}, errors.New("expired"), 401},
		{"duplicate cookies", "GET", "", []string{"one", "two"}, nil, 401},
	} {
		t.Run(test.name, func(t *testing.T) {
			verifier := &fakeVerifier{principal: models.Principal{UserID: testMiddlewareUserID}, err: test.verifyErr}
			handler := NewAuthentication(verifier, CookieAuthenticationConfig{Name: "custom_access", AllowedOrigin: "https://app.example"}).Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if _, ok := PrincipalFromContext(r.Context()); !ok {
					t.Fatal("missing principal")
				}
				w.WriteHeader(204)
			}))
			request := httptest.NewRequest(test.method, "/protected", nil)
			request.Header.Set("Origin", test.origin)
			request.Header.Set("Authorization", "Bearer fallback-must-not-override-cookie")
			for _, value := range test.cookies {
				request.AddCookie(&http.Cookie{Name: "custom_access", Value: value})
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status=%d, want %d", response.Code, test.wantStatus)
			}
			if response.Code == 204 && verifier.raw != "valid-token" {
				t.Fatal("access cookie was not verified")
			}
		})
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
