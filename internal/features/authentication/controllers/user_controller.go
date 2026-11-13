package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	authmodels "github.com/harzeezco/zeexco_commerce/internal/features/authentication/models"
	"github.com/harzeezco/zeexco_commerce/internal/features/authentication/schemas"
	"github.com/harzeezco/zeexco_commerce/internal/features/authentication/services"
)

const maxRegistrationBodyBytes int64 = 16 << 10

type RegistrationService interface {
	Register(context.Context, services.RegisterCommand) (*authmodels.Registration, error)
}

type AuthController struct {
	registerService RegistrationService
}

func NewAuthController(registerService RegistrationService) *AuthController {
	return &AuthController{registerService: registerService}
}

func (c *AuthController) Register(w http.ResponseWriter, r *http.Request) {
	request, err := decodeRegisterRequest(w, r)
	if err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			writeError(w, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE", "request body is too large", "")
			return
		}
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "request body must be a single valid JSON object", "")
		return
	}

	command := services.RegisterCommand{
		FirstName:    request.FirstName,
		LastName:     request.LastName,
		BusinessName: request.BusinessName,
		Email:        request.Email,
		Password:     request.Password,
	}
	registration, err := c.registerService.Register(r.Context(), command)
	if err != nil {
		var validationError *services.ValidationError
		switch {
		case errors.As(err, &validationError):
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", validationError.Message, validationError.Field)
		case errors.Is(err, authmodels.ErrEmailAlreadyRegistered):
			writeError(w, http.StatusConflict, "EMAIL_ALREADY_REGISTERED", "An account with this email already exists", "email")
		default:
			writeError(w, http.StatusInternalServerError, "REGISTRATION_FAILED", "registration could not be completed", "")
		}
		return
	}

	writeJSON(w, http.StatusAccepted, schemas.RegisterResponse{
		User: schemas.RegisteredUser{
			ID:        registration.User.ID,
			FirstName: registration.User.FirstName,
			LastName:  registration.User.LastName,
			Email:     registration.User.Email,
			Status:    string(registration.User.Status),
		},
		Business: schemas.RegisteredBusiness{
			ID:     registration.Business.ID,
			Name:   registration.Business.Name,
			Status: string(registration.Business.Status),
		},
		NextStep: services.NextStepVerifyEmail,
	})
}

func decodeRegisterRequest(w http.ResponseWriter, r *http.Request) (schemas.RegisterRequest, error) {
	var request schemas.RegisterRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRegistrationBodyBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return schemas.RegisterRequest{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return schemas.RegisterRequest{}, errors.New("request contains multiple JSON values")
	}
	return request, nil
}

func writeError(w http.ResponseWriter, status int, code, message, field string) {
	writeJSON(w, status, schemas.ErrorResponse{Error: schemas.APIError{
		Code: code, Message: message, Field: field,
	}})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
