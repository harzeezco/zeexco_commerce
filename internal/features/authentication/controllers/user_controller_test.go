package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authmodels "github.com/harzeezco/zeexco_commerce/internal/features/authentication/models"
	"github.com/harzeezco/zeexco_commerce/internal/features/authentication/schemas"
	"github.com/harzeezco/zeexco_commerce/internal/features/authentication/services"
	businessmodels "github.com/harzeezco/zeexco_commerce/internal/features/business/models"
)

func TestRegisterReturnsBusinessAndNextOnboardingStep(t *testing.T) {
	service := &fakeRegistrationService{registration: controllerTestRegistration()}
	body := `{
		"first_name":"Ada",
		"last_name":"Lovelace",
		"business_name":"Analytical Commerce",
		"email":"ada@example.com",
		"password":"correct horse battery staple"
	}`
	response := performRegistrationRequest(service, body)

	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var payload schemas.RegisterResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.User.ID == "" || payload.Business.ID == "" ||
		payload.User.Status != "PENDING" || payload.Business.Status != "ACTIVE" ||
		payload.NextStep != services.NextStepVerifyEmail {
		t.Fatalf("response = %#v", payload)
	}
	if strings.Contains(response.Body.String(), "password") {
		t.Fatalf("response contains password data: %s", response.Body.String())
	}
	if service.command.BusinessName != "Analytical Commerce" || service.command.Password == "" {
		t.Fatalf("service command = %#v", service.command)
	}
}

func TestWebRegisterRejectsWhatsAppOnboardingMode(t *testing.T) {
	service := &fakeRegistrationService{}
	response := performRegistrationRequest(service, `{
		"first_name":"Ada","last_name":"Lovelace","business_name":"Commerce",
		"email":"ada@example.com","password":"correct horse battery staple",
		"onboarding_token":"token"
	}`)
	assertAPIError(t, response, http.StatusBadRequest, "INVALID_JSON")
	if service.calls != 0 {
		t.Fatal("web registration accepted WhatsApp onboarding mode")
	}
}

func TestRegisterRejectsClientSuppliedWhatsAppIdentity(t *testing.T) {
	service := &fakeRegistrationService{}
	response := performRegistrationRequest(service, `{
		"first_name":"Ada","last_name":"Lovelace","business_name":"Commerce",
		"email":"ada@example.com","password":"correct horse battery staple",
		"whatsapp_provider_user_id":"attacker-selected-id"
	}`)
	assertAPIError(t, response, http.StatusBadRequest, "INVALID_JSON")
	if service.calls != 0 {
		t.Fatal("registration service called with client-selected WhatsApp identity")
	}
}

func TestRegisterRejectsMalformedUnknownAndMultipleJSON(t *testing.T) {
	tests := []string{
		`{"first_name":`,
		`{"first_name":"Ada","unknown":"value"}`,
		`{} {}`,
	}
	for _, body := range tests {
		service := &fakeRegistrationService{}
		response := performRegistrationRequest(service, body)
		assertAPIError(t, response, http.StatusBadRequest, "INVALID_JSON")
		if service.calls != 0 {
			t.Fatal("service called for invalid JSON")
		}
	}
}

func TestRegisterMapsValidationDuplicateAndInternalErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "validation", err: &services.ValidationError{Field: "email", Message: "email address is invalid"}, wantStatus: http.StatusBadRequest, wantCode: "VALIDATION_ERROR"},
		{name: "duplicate", err: authmodels.ErrEmailAlreadyRegistered, wantStatus: http.StatusConflict, wantCode: "EMAIL_ALREADY_REGISTERED"},
		{name: "internal", err: errors.New("database internals must remain hidden"), wantStatus: http.StatusInternalServerError, wantCode: "REGISTRATION_FAILED"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := performRegistrationRequest(&fakeRegistrationService{err: test.err}, `{}`)
			assertAPIError(t, response, test.wantStatus, test.wantCode)
			if strings.Contains(response.Body.String(), "database internals") {
				t.Fatal("response leaked internal error")
			}
		})
	}
}

func TestRegisterRejectsOversizedBody(t *testing.T) {
	service := &fakeRegistrationService{}
	response := performRegistrationRequest(service, `{"padding":"`+strings.Repeat("x", int(maxRegistrationBodyBytes))+`"}`)
	assertAPIError(t, response, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE")
}

func performRegistrationRequest(service RegistrationService, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(body))
	response := httptest.NewRecorder()
	NewAuthController(service).Register(response, request)
	return response
}

func assertAPIError(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("status = %d, want %d, body = %s", response.Code, status, response.Body.String())
	}
	var payload schemas.ErrorResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if payload.Error.Code != code {
		t.Fatalf("error code = %q, want %q", payload.Error.Code, code)
	}
}

func controllerTestRegistration() *authmodels.Registration {
	now := time.Unix(1800000000, 0).UTC()
	return &authmodels.Registration{
		User: authmodels.User{
			ID: "00000000-0000-4000-8000-000000000001", FirstName: "Ada", LastName: "Lovelace",
			Email: "ada@example.com", Status: authmodels.UserStatusPending, CreatedAt: now, UpdatedAt: now,
		},
		Business: businessmodels.Business{
			ID: "00000000-0000-4000-8000-000000000002", Name: "Analytical Commerce",
			Status: businessmodels.StatusActive, CreatedAt: now, UpdatedAt: now,
		},
		Membership: businessmodels.Membership{
			ID:         "00000000-0000-4000-8000-000000000003",
			BusinessID: "00000000-0000-4000-8000-000000000002",
			UserID:     "00000000-0000-4000-8000-000000000001", Role: businessmodels.RoleOwner, CreatedAt: now,
		},
	}
}

type fakeRegistrationService struct {
	registration *authmodels.Registration
	err          error
	command      services.RegisterCommand
	calls        int
}

func (s *fakeRegistrationService) Register(_ context.Context, command services.RegisterCommand) (*authmodels.Registration, error) {
	s.calls++
	s.command = command
	return s.registration, s.err
}
