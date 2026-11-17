package flows

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	authmodels "github.com/harzeezco/zeexco_commerce/internal/features/authentication/models"
	"github.com/harzeezco/zeexco_commerce/internal/features/whatsapp/onboarding"
)

const (
	DataAPIVersion     = "3.0"
	RegistrationScreen = "REGISTRATION"
	SuccessScreen      = "SUCCESS"
)

var (
	ErrInvalidPayload          = errors.New("invalid WhatsApp Flow payload")
	ErrUnsupportedAction       = errors.New("unsupported WhatsApp Flow action")
	ErrRegistrationUnavailable = errors.New("WhatsApp Flow registration is unavailable")
)

type Payload struct {
	Version   string          `json:"version"`
	Action    string          `json:"action"`
	Screen    string          `json:"screen,omitempty"`
	Data      json.RawMessage `json:"data,omitempty"`
	FlowToken string          `json:"flow_token,omitempty"`
}

type Registrar interface {
	Complete(context.Context, string, onboarding.RegisterCommand) (onboarding.Completion, error)
}

type ActionHandler struct {
	registrar Registrar
}

func NewActionHandler(registrar Registrar) *ActionHandler {
	return &ActionHandler{registrar: registrar}
}

func (h *ActionHandler) Handle(ctx context.Context, raw []byte) (any, error) {
	var payload Payload
	if err := decodeStrict(raw, &payload); err != nil || payload.Version != DataAPIVersion || payload.Action == "" {
		return nil, ErrInvalidPayload
	}
	switch payload.Action {
	case "ping":
		return map[string]any{"data": map[string]string{"status": "active"}}, nil
	case "INIT":
		return map[string]any{
			"screen": RegistrationScreen,
			"data":   map[string]any{"has_error": false, "error_message": ""},
		}, nil
	case "data_exchange":
		return h.register(ctx, payload)
	default:
		return nil, ErrUnsupportedAction
	}
}

type registrationData struct {
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name"`
	BusinessName string `json:"business_name"`
	Email        string `json:"email,omitempty"`
}

func (h *ActionHandler) register(ctx context.Context, payload Payload) (any, error) {
	if h.registrar == nil {
		return nil, ErrRegistrationUnavailable
	}
	if payload.Screen != RegistrationScreen || payload.FlowToken == "" || len(payload.Data) == 0 {
		return nil, ErrInvalidPayload
	}
	var data registrationData
	if err := decodeStrict(payload.Data, &data); err != nil {
		return nil, ErrInvalidPayload
	}
	_, err := h.registrar.Complete(ctx, payload.FlowToken, onboarding.RegisterCommand{
		FirstName: data.FirstName, LastName: data.LastName,
		BusinessName: data.BusinessName, Email: data.Email,
	})
	if err != nil {
		return nil, mapRegistrationError(err)
	}
	return map[string]any{
		"screen": SuccessScreen,
		"data": map[string]any{
			"extension_message_response": map[string]any{
				"params": map[string]string{"flow_token": payload.FlowToken},
			},
		},
	}, nil
}

func decodeStrict(raw []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return ErrInvalidPayload
		}
		return err
	}
	return nil
}

type DomainError struct {
	Code    string
	Message string
	Field   string
	Status  int
}

func (e *DomainError) Error() string { return e.Code }

func mapRegistrationError(err error) error {
	var validation *onboarding.ValidationError
	switch {
	case errors.As(err, &validation):
		return &DomainError{Code: "VALIDATION_ERROR", Message: validation.Message, Field: validation.Field, Status: 200}
	case errors.Is(err, authmodels.ErrEmailAlreadyRegistered):
		return &DomainError{Code: "EMAIL_ALREADY_REGISTERED", Message: "An account with this email already exists.", Field: "email", Status: 200}
	case errors.Is(err, onboarding.ErrIdentityConflict):
		return &DomainError{Code: "WHATSAPP_IDENTITY_CONFLICT", Message: "This WhatsApp identity is already linked.", Status: 200}
	case errors.Is(err, onboarding.ErrSessionExpired):
		return &DomainError{Code: "ONBOARDING_EXPIRED", Message: "This registration session has expired.", Status: 427}
	case errors.Is(err, onboarding.ErrSessionConsumed):
		return &DomainError{Code: "ONBOARDING_CONSUMED", Message: "This registration session has already been used.", Status: 427}
	case errors.Is(err, onboarding.ErrMalformedToken), errors.Is(err, onboarding.ErrInvalidSession):
		return &DomainError{Code: "ONBOARDING_INVALID", Message: "This registration session is invalid.", Status: 427}
	default:
		return fmt.Errorf("complete Flow registration: %w", err)
	}
}

func DomainErrorResponse(err *DomainError) map[string]any {
	if err.Status == 427 {
		return map[string]any{"error_msg": err.Message}
	}
	data := map[string]any{"error_code": err.Code, "error_message": err.Message}
	data["has_error"] = true
	if err.Field != "" {
		data["error_field"] = err.Field
	}
	return map[string]any{"screen": RegistrationScreen, "data": data}
}
