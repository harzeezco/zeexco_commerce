package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	authmiddleware "github.com/harzeezco/zeexco_commerce/internal/features/authentication/middleware"
	"github.com/harzeezco/zeexco_commerce/internal/features/authentication/models"
	"github.com/harzeezco/zeexco_commerce/internal/features/authentication/schemas"
	"github.com/harzeezco/zeexco_commerce/internal/features/authentication/services"
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
	cookies := response.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Value != "new-refresh" || !cookies[0].HttpOnly || !cookies[0].Secure || cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatalf("cookies=%#v", cookies)
	}
	if bytes.Contains(response.Body.Bytes(), []byte("new-refresh")) {
		t.Fatal("response body exposed refresh token")
	}
	var payload schemas.AuthenticationResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.AccessToken != "access-token" || payload.TokenType != "Bearer" || len(payload.Businesses) != 1 {
		t.Fatalf("payload=%#v", payload)
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
	if cookies := response.Result().Cookies(); len(cookies) != 1 || cookies[0].Value != "replacement-refresh" {
		t.Fatalf("cookies=%#v", cookies)
	}

	service.refreshErr = services.ErrInvalidRefresh
	response = httptest.NewRecorder()
	controller.Refresh(response, request)
	assertAPIError(t, response, http.StatusUnauthorized, "INVALID_REFRESH_SESSION")
	if cookies := response.Result().Cookies(); len(cookies) != 1 || cookies[0].MaxAge != -1 {
		t.Fatalf("invalid refresh did not clear cookie: %#v", cookies)
	}
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
	if cookies := response.Result().Cookies(); len(cookies) != 1 || cookies[0].MaxAge != -1 {
		t.Fatalf("logout cookie=%#v", cookies)
	}
}

func TestAuthenticationControllerMeUsesMiddlewarePrincipal(t *testing.T) {
	service := &fakeAuthenticationControllerService{userContext: controllerAuthenticationResult("").Context}
	controller := NewAuthenticationController(service, testCookieConfig())
	verifier := controllerTokenVerifier{principal: models.Principal{UserID: controllerAuthUserID}}
	handler := authmiddleware.NewAuthentication(verifier).Require(http.HandlerFunc(controller.Me))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || service.principal.UserID != controllerAuthUserID {
		t.Fatalf("status=%d principal=%#v", response.Code, service.principal)
	}
}

const controllerAuthUserID = "00000000-0000-4000-8000-000000000001"

func testCookieConfig() RefreshCookieConfig {
	return RefreshCookieConfig{
		Name: "zeexco_refresh", Path: "/api/v1/auth", Secure: true,
		SameSite: http.SameSiteLaxMode, Lifetime: 30 * 24 * time.Hour,
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
	s.principal = principal
	return s.userContext, nil
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
