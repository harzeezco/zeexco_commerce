package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"
	"time"

	authmiddleware "github.com/harzeezco/zeexco_commerce/internal/features/authentication/middleware"
	"github.com/harzeezco/zeexco_commerce/internal/features/authentication/models"
	"github.com/harzeezco/zeexco_commerce/internal/features/authentication/schemas"
	"github.com/harzeezco/zeexco_commerce/internal/features/authentication/services"
	"github.com/harzeezco/zeexco_commerce/internal/platform/httpapi"
)

func TestAuthenticationControllerLoginSetsSecureRefreshCookie(t *testing.T) {
	service := &fakeAuthenticationControllerService{result: controllerAuthenticationResult("new-refresh")}
	controller := NewAuthenticationController(service, testCookieConfig())
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"email":"merchant@example.com","password":"correct-password"}`))
	response := httptest.NewRecorder()
	controller.Login(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("Pragma") != "no-cache" {
		t.Fatalf("authentication cache headers = %#v", response.Header())
	}
	assertSessionCookies(t, response, "access-token", "new-refresh", false)
	if bytes.Contains(response.Body.Bytes(), []byte("new-refresh")) {
		t.Fatal("response body exposed refresh token")
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload) != 2 || string(payload["status_code"]) != "200" || string(payload["message"]) != `"Login successful. Welcome back"` {
		t.Fatalf("payload=%#v", payload)
	}
}

func TestAuthenticationControllerPendingLoginReturnsVerificationRequired(t *testing.T) {
	service := &fakeAuthenticationControllerService{loginErr: services.ErrEmailVerificationRequired}
	controller := NewAuthenticationController(service, testCookieConfig())
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"email":"merchant@example.com","password":"correct-password"}`))
	response := httptest.NewRecorder()
	controller.Login(response, request)
	assertAPIError(t, response, http.StatusForbidden, "EMAIL_VERIFICATION_REQUIRED")
	if len(response.Result().Cookies()) != 0 {
		t.Fatal("pending account received session cookies")
	}
}

func TestAuthenticationControllerUsesGenericInvalidCredentials(t *testing.T) {
	service := &fakeAuthenticationControllerService{loginErr: services.ErrInvalidCredentials}
	controller := NewAuthenticationController(service, testCookieConfig())
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"email":"unknown@example.com","password":"wrong-password"}`))
	response := httptest.NewRecorder()
	controller.Login(response, request)
	assertAPIError(t, response, http.StatusUnauthorized, "INVALID_CREDENTIALS")
}

func TestAuthenticationControllerRefreshRotatesCookieAndRejectsInvalid(t *testing.T) {
	service := &fakeAuthenticationControllerService{result: controllerAuthenticationResult("replacement-refresh")}
	controller := NewAuthenticationController(service, testCookieConfig())
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	request.AddCookie(&http.Cookie{Name: "zeexco_refresh", Value: "old-refresh"})
	response := httptest.NewRecorder()
	controller.Refresh(response, request)
	if response.Code != http.StatusOK || service.refreshToken != "old-refresh" {
		t.Fatalf("status=%d refresh=%q", response.Code, service.refreshToken)
	}
	assertSessionCookies(t, response, "access-token", "replacement-refresh", false)
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode refresh response: %v", err)
	}
	if len(payload) != 2 || string(payload["status_code"]) != "200" || string(payload["message"]) != `"Session refreshed successfully."` {
		t.Fatalf("refresh payload=%#v", payload)
	}

	service.refreshErr = services.ErrInvalidRefresh
	response = httptest.NewRecorder()
	controller.Refresh(response, request)
	assertAPIError(t, response, http.StatusUnauthorized, "INVALID_REFRESH_SESSION")
	assertSessionCookies(t, response, "", "", true)
}

func TestAuthenticationControllerLogoutRevokesAndClearsCookie(t *testing.T) {
	service := &fakeAuthenticationControllerService{}
	controller := NewAuthenticationController(service, testCookieConfig())
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	request.AddCookie(&http.Cookie{Name: "zeexco_refresh", Value: "refresh-token"})
	response := httptest.NewRecorder()
	controller.Logout(response, request)
	if response.Code != http.StatusNoContent || service.logoutToken != "refresh-token" {
		t.Fatalf("status=%d token=%q", response.Code, service.logoutToken)
	}
	assertSessionCookies(t, response, "", "", true)
}

func TestAuthenticationControllerMeUsesMiddlewarePrincipal(t *testing.T) {
	service := &fakeAuthenticationControllerService{userContext: controllerAuthenticationResult("").Context}
	controller := NewAuthenticationController(service, testCookieConfig())
	verifier := controllerTokenVerifier{principal: models.Principal{UserID: controllerAuthUserID}}
	handler := authmiddleware.NewAuthentication(verifier).Require(http.HandlerFunc(controller.Me))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	request.AddCookie(&http.Cookie{Name: "zeexco_access", Value: "access-token"})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || service.principal.UserID != controllerAuthUserID {
		t.Fatalf("status=%d principal=%#v", response.Code, service.principal)
	}
}

func TestSessionReturnsCurrentAuthorizationContext(t *testing.T) {
	for _, test := range []struct {
		name       string
		cookie     string
		validToken bool
		status     models.UserStatus
		loadErr    error
		wantStatus int
	}{
		{"active", "access-token", true, models.UserStatusActive, nil, 200},
		{"missing cookie", "", true, models.UserStatusActive, nil, 401},
		{"invalid or expired cookie", "invalid", false, models.UserStatusActive, nil, 401},
		{"pending account", "access-token", true, models.UserStatusPending, nil, 403},
		{"suspended account", "access-token", true, models.UserStatusSuspended, nil, 403},
		{"deleted account", "access-token", true, models.UserStatusActive, models.ErrCredentialsNotFound, 401},
		{"database failure", "access-token", true, models.UserStatusActive, errors.New("private database details"), 500},
	} {
		t.Run(test.name, func(t *testing.T) {
			userContext := controllerAuthenticationResult("").Context
			userContext.User.Status = test.status
			service := &fakeAuthenticationControllerService{userContext: userContext, meErr: test.loadErr}
			controller := NewAuthenticationController(service, testCookieConfig())
			verifier := controllerTokenVerifier{}
			if test.validToken {
				verifier.principal = models.Principal{UserID: controllerAuthUserID}
			}
			handler := authmiddleware.NewAuthentication(verifier).Require(http.HandlerFunc(controller.Session))
			request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session?user_id=untrusted-id", nil)
			if test.cookie != "" {
				request.AddCookie(&http.Cookie{Name: "zeexco_access", Value: test.cookie})
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status=%d want=%d", response.Code, test.wantStatus)
			}
			if response.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("session response is cacheable")
			}
			if len(response.Result().Cookies()) != 0 {
				t.Fatal("GET session must not renew or rotate cookies")
			}
			for _, secret := range []string{"access-token", "private database details", "password", "refresh_token"} {
				if bytes.Contains(response.Body.Bytes(), []byte(secret)) {
					t.Fatalf("session exposed %q", secret)
				}
			}
			if test.cookie == "" || !test.validToken {
				if service.meCalls != 0 {
					t.Fatal("unauthenticated request loaded user context")
				}
				return
			}
			if service.principal.UserID != controllerAuthUserID {
				t.Fatal("session did not use the authenticated principal")
			}
			if response.Code == 200 {
				var payload schemas.SessionResponse
				if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
					t.Fatal(err)
				}
				if payload.StatusCode != 200 || payload.Message != "Session retrieved successfully." || payload.User.ID != controllerAuthUserID || len(payload.Businesses) != 1 || payload.Businesses[0].MembershipRole != "OWNER" || payload.Businesses[0].ID != userContext.Businesses[0].Business.ID {
					t.Fatalf("incorrect session context: %#v", payload)
				}
			}
		})
	}
}

const controllerAuthUserID = "00000000-0000-4000-8000-000000000001"

func TestBrowserCookieSessionLifecycle(t *testing.T) {
	const origin = "https://app.example"
	service := &fakeAuthenticationControllerService{result: controllerAuthenticationResult("refresh-token")}
	service.userContext = service.result.Context
	service.userContext.User.Status = models.UserStatusActive
	controller := NewAuthenticationController(service, testCookieConfig())
	mux := http.NewServeMux()
	mux.Handle("POST /api/v1/auth/login", httpapi.RequireOrigin(origin, http.HandlerFunc(controller.Login)))
	mux.Handle("POST /api/v1/auth/refresh", httpapi.RequireOrigin(origin, http.HandlerFunc(controller.Refresh)))
	mux.Handle("POST /api/v1/auth/logout", httpapi.RequireOrigin(origin, http.HandlerFunc(controller.Logout)))
	verifier := controllerTokenVerifier{principal: models.Principal{UserID: controllerAuthUserID}}
	mux.Handle("GET /api/v1/auth/session", authmiddleware.NewAuthentication(verifier).Require(http.HandlerFunc(controller.Session)))
	mux.Handle("GET /api/v1/products", authmiddleware.NewAuthentication(verifier).Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := r.Cookie("zeexco_refresh"); err == nil {
			t.Error("refresh cookie escaped its auth path")
		}
		cookie, err := r.Cookie("zeexco_access")
		if err != nil || cookie.Value != "access-token" {
			t.Error("access cookie missing from protected API request")
		}
		w.WriteHeader(http.StatusNoContent)
	})))
	server := httptest.NewTLSServer(mux)
	defer server.Close()
	client := server.Client()
	client.Jar, _ = cookiejar.New(nil)
	request := func(method, path, requestOrigin, body string, wantStatus int) {
		t.Helper()
		req, err := http.NewRequest(method, server.URL+path, bytes.NewBufferString(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Origin", requestOrigin)
		req.Header.Set("Content-Type", "application/json")
		response, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		content, _ := io.ReadAll(response.Body)
		if response.StatusCode != wantStatus {
			t.Fatalf("%s %s: status=%d want=%d", method, path, response.StatusCode, wantStatus)
		}
		if path != "/api/v1/products" && response.StatusCode == http.StatusOK {
			var payload map[string]json.RawMessage
			wantFields := 2
			if path == "/api/v1/auth/session" {
				wantFields = 4
			}
			if err := json.Unmarshal(content, &payload); err != nil || len(payload) != wantFields || string(payload["status_code"]) != "200" || len(payload["message"]) == 0 {
				t.Fatal("session response contains unexpected data")
			}
		}
	}
	request("GET", "/api/v1/products", "", "", 401)
	request("GET", "/api/v1/auth/session", "", "", 401)
	request("POST", "/api/v1/auth/login", "https://attacker.example", `{"email":"merchant@example.com","password":"correct-password"}`, 403)
	request("POST", "/api/v1/auth/login", origin, `{"email":"merchant@example.com","password":"correct-password"}`, 200)
	request("GET", "/api/v1/auth/session", "", "", 200)
	request("GET", "/api/v1/products", "", "", 204)
	request("POST", "/api/v1/auth/refresh", "https://attacker.example", "", 403)
	request("POST", "/api/v1/auth/refresh", origin, "", 200)
	request("GET", "/api/v1/auth/session", "", "", 200)
	if service.refreshToken != "refresh-token" {
		t.Fatal("refresh cookie was not sent automatically")
	}
	request("POST", "/api/v1/auth/logout", origin, "", 204)
	request("GET", "/api/v1/auth/session", "", "", 401)
	request("GET", "/api/v1/products", "", "", 401)
}

func testCookieConfig() SessionCookieConfig {
	return SessionCookieConfig{
		AccessName: "zeexco_access",
		Name:       "zeexco_refresh", Path: "/api/v1/auth", Secure: true,
		SameSite: http.SameSiteLaxMode, Lifetime: 30 * 24 * time.Hour,
	}
}

func assertSessionCookies(t *testing.T, response *httptest.ResponseRecorder, access, refresh string, cleared bool) {
	t.Helper()
	cookies := response.Result().Cookies()
	if len(cookies) != 2 {
		t.Fatalf("cookie count=%d, want 2", len(cookies))
	}
	want := map[string]struct {
		value, path string
		maxAge      int
	}{
		"zeexco_access":  {access, "/", 900},
		"zeexco_refresh": {refresh, "/api/v1/auth", 30 * 24 * 60 * 60},
	}
	for _, cookie := range cookies {
		expected, ok := want[cookie.Name]
		if !ok || cookie.Value != expected.value || cookie.Path != expected.path || !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode {
			t.Fatalf("invalid session cookie attributes for %q", cookie.Name)
		}
		if cleared {
			if cookie.MaxAge != -1 || !cookie.Expires.Before(time.Now()) {
				t.Fatal("session cookie was not expired")
			}
		} else if cookie.MaxAge != expected.maxAge || !cookie.Expires.After(time.Now()) {
			t.Fatalf("incorrect cookie lifetime for %q", cookie.Name)
		}
		delete(want, cookie.Name)
	}
}

func controllerAuthenticationResult(refresh string) services.AuthenticationResult {
	registration := controllerTestRegistration()
	return services.AuthenticationResult{
		AccessToken: "access-token", ExpiresIn: 900, RefreshToken: refresh,
		Context: models.UserContext{
			User: registration.User,
			Businesses: []models.BusinessContext{{
				Business: registration.Business, Membership: registration.Membership,
			}},
		},
	}
}

type fakeAuthenticationControllerService struct {
	result       services.AuthenticationResult
	loginErr     error
	refreshErr   error
	refreshToken string
	logoutToken  string
	userContext  models.UserContext
	principal    models.Principal
	meErr        error
	meCalls      int
}

func (s *fakeAuthenticationControllerService) Login(context.Context, services.LoginCommand) (services.AuthenticationResult, error) {
	return s.result, s.loginErr
}
func (s *fakeAuthenticationControllerService) Refresh(_ context.Context, raw string) (services.AuthenticationResult, error) {
	s.refreshToken = raw
	return s.result, s.refreshErr
}
func (s *fakeAuthenticationControllerService) Logout(_ context.Context, raw string) error {
	s.logoutToken = raw
	return nil
}
func (s *fakeAuthenticationControllerService) Me(_ context.Context, principal models.Principal) (models.UserContext, error) {
	s.meCalls++
	s.principal = principal
	return s.userContext, s.meErr
}

type controllerTokenVerifier struct {
	principal models.Principal
}

func (v controllerTokenVerifier) Verify(string) (models.Principal, error) {
	if v.principal.UserID == "" {
		return models.Principal{}, errors.New("invalid")
	}
	return v.principal, nil
}
