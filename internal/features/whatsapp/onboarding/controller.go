package onboarding

import (
	"context"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"net/http"
	"time"

	authmodels "github.com/harzeezco/zeexco_commerce/internal/features/authentication/models"
	authschemas "github.com/harzeezco/zeexco_commerce/internal/features/authentication/schemas"
)

const maxOnboardingFormBytes int64 = 32 << 10

type OnboardingService interface {
	Validate(context.Context, string) (Session, error)
	Complete(context.Context, string, RegisterCommand) (Completion, error)
	RetryCompletion(context.Context, string)
	ReturnURL() string
}

type Controller struct {
	service OnboardingService
}

func NewController(service OnboardingService) *Controller { return &Controller{service: service} }

type validationResponse struct {
	Valid     bool      `json:"valid"`
	Status    string    `json:"status"`
	ExpiresAt time.Time `json:"expires_at"`
}

type apiErrorResponse struct {
	Error apiError `json:"error"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
}

func (c *Controller) ValidateAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	values, present := r.URL.Query()["token"]
	if !present || len(values) != 1 || values[0] == "" {
		writeAPIError(w, http.StatusBadRequest, "ONBOARDING_TOKEN_REQUIRED", "An onboarding token is required.")
		return
	}
	session, err := c.service.Validate(r.Context(), values[0])
	if err != nil {
		switch {
		case errors.Is(err, ErrMalformedToken):
			writeAPIError(w, http.StatusBadRequest, "ONBOARDING_TOKEN_INVALID", "This registration link is invalid.")
		case errors.Is(err, ErrInvalidSession):
			writeAPIError(w, http.StatusNotFound, "ONBOARDING_TOKEN_INVALID", "This registration link is invalid.")
		case errors.Is(err, ErrSessionExpired):
			writeAPIError(w, http.StatusGone, "ONBOARDING_TOKEN_EXPIRED", "This registration link has expired.")
		case errors.Is(err, ErrSessionConsumed):
			writeAPIError(w, http.StatusConflict, "ONBOARDING_TOKEN_CONSUMED", "This registration link has already been used.")
		default:
			writeAPIError(w, http.StatusInternalServerError, "ONBOARDING_VALIDATION_FAILED", "The registration link could not be validated.")
		}
		return
	}
	writeAPIJSON(w, http.StatusOK, validationResponse{Valid: true, Status: "PENDING", ExpiresAt: session.ExpiresAt})
}

func (c *Controller) RegisterAPI(w http.ResponseWriter, r *http.Request) {
	var request WhatsAppRegisterRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxOnboardingFormBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			writeAPIError(w, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE", "Request body is too large.")
			return
		}
		writeAPIError(w, http.StatusBadRequest, "INVALID_JSON", "Request body must be a single valid JSON object.")
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeAPIError(w, http.StatusBadRequest, "INVALID_JSON", "Request body must be a single valid JSON object.")
		return
	}
	if request.OnboardingToken == "" {
		writeAPIFieldError(w, http.StatusBadRequest, "ONBOARDING_TOKEN_REQUIRED", "An onboarding token is required.", "onboarding_token")
		return
	}
	completion, err := c.service.Complete(r.Context(), request.OnboardingToken, RegisterCommand{
		FirstName: request.FirstName, LastName: request.LastName,
		BusinessName: request.BusinessName, Email: request.Email,
	})
	if err != nil {
		c.writeRegistrationError(w, err)
		return
	}
	registration := completion.Registration
	writeAPIJSON(w, http.StatusCreated, authschemas.RegisterResponse{
		User: authschemas.RegisteredUser{
			ID: registration.User.ID, FirstName: registration.User.FirstName,
			LastName: registration.User.LastName, Email: registration.User.Email,
			Status: string(registration.User.Status),
		},
		Business: authschemas.RegisteredBusiness{
			ID: registration.Business.ID, Name: registration.Business.Name,
			Status: string(registration.Business.Status),
		},
		NextStep: "ADD_FIRST_PRODUCT",
	})
}

func (c *Controller) writeRegistrationError(w http.ResponseWriter, err error) {
	var validationError *ValidationError
	switch {
	case errors.As(err, &validationError):
		writeAPIFieldError(w, http.StatusBadRequest, "VALIDATION_ERROR", validationError.Message, validationError.Field)
	case errors.Is(err, authmodels.ErrEmailAlreadyRegistered):
		writeAPIFieldError(w, http.StatusConflict, "EMAIL_ALREADY_REGISTERED", "An account with this email already exists.", "email")
	case errors.Is(err, ErrMalformedToken), errors.Is(err, ErrInvalidSession):
		writeAPIFieldError(w, http.StatusBadRequest, "ONBOARDING_TOKEN_INVALID", "This registration link is invalid.", "onboarding_token")
	case errors.Is(err, ErrSessionExpired):
		writeAPIFieldError(w, http.StatusGone, "ONBOARDING_TOKEN_EXPIRED", "This registration link has expired.", "onboarding_token")
	case errors.Is(err, ErrSessionConsumed):
		writeAPIFieldError(w, http.StatusConflict, "ONBOARDING_TOKEN_CONSUMED", "This registration link has already been used.", "onboarding_token")
	case errors.Is(err, ErrIdentityConflict):
		writeAPIFieldError(w, http.StatusConflict, "WHATSAPP_IDENTITY_CONFLICT", "This WhatsApp identity is already linked.", "onboarding_token")
	default:
		writeAPIError(w, http.StatusInternalServerError, "REGISTRATION_FAILED", "Registration could not be completed.")
	}
}

func writeAPIError(w http.ResponseWriter, status int, code, message string) {
	writeAPIJSON(w, status, apiErrorResponse{Error: apiError{Code: code, Message: message}})
}

func writeAPIFieldError(w http.ResponseWriter, status int, code, message, field string) {
	writeAPIJSON(w, status, apiErrorResponse{Error: apiError{Code: code, Message: message, Field: field}})
}

func writeAPIJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

type pageData struct {
	Token        string
	Error        string
	FirstName    string
	LastName     string
	Email        string
	BusinessName string
	ReturnURL    string
}

func (c *Controller) Page(w http.ResponseWriter, r *http.Request) {
	setPageSecurityHeaders(w)
	token, ok := singleToken(r)
	if !ok {
		renderExpired(w)
		return
	}
	session, err := c.service.Validate(r.Context(), token)
	if errors.Is(err, ErrSessionConsumed) {
		if session.LinkedUserID != nil {
			c.service.RetryCompletion(r.Context(), *session.LinkedUserID)
		}
		renderSuccess(w, c.service.ReturnURL())
		return
	}
	if err != nil {
		renderExpired(w)
		return
	}
	renderForm(w, http.StatusOK, pageData{Token: token})
}

func (c *Controller) Submit(w http.ResponseWriter, r *http.Request) {
	setPageSecurityHeaders(w)
	r.Body = http.MaxBytesReader(w, r.Body, maxOnboardingFormBytes)
	if err := r.ParseForm(); err != nil {
		renderForm(w, http.StatusBadRequest, pageData{Error: "Please check the form and try again."})
		return
	}
	data := pageData{
		Token: r.PostForm.Get("token"), FirstName: r.PostForm.Get("first_name"),
		LastName: r.PostForm.Get("last_name"), Email: r.PostForm.Get("email"),
		BusinessName: r.PostForm.Get("business_name"),
	}
	_, err := c.service.Complete(r.Context(), data.Token, RegisterCommand{
		FirstName: data.FirstName, LastName: data.LastName, Email: data.Email,
		BusinessName: data.BusinessName,
	})
	if err != nil {
		var validationError *ValidationError
		switch {
		case errors.As(err, &validationError):
			data.Error = validationError.Message
			renderForm(w, http.StatusBadRequest, data)
		case errors.Is(err, authmodels.ErrEmailAlreadyRegistered):
			data.Error = "An account with this email already exists. For your security, sign in and use a future authenticated linking flow."
			renderForm(w, http.StatusConflict, data)
		case errors.Is(err, ErrMalformedToken), errors.Is(err, ErrInvalidSession),
			errors.Is(err, ErrSessionExpired), errors.Is(err, ErrSessionConsumed):
			renderExpired(w)
		default:
			data.Error = "Setup could not be completed. Please try again."
			renderForm(w, http.StatusInternalServerError, data)
		}
		return
	}
	renderSuccess(w, c.service.ReturnURL())
}

func singleToken(r *http.Request) (string, bool) {
	values, ok := r.URL.Query()["t"]
	return firstSingle(values, ok)
}

func firstSingle(values []string, ok bool) (string, bool) {
	returnValue := ""
	if ok && len(values) == 1 {
		returnValue = values[0]
	}
	return returnValue, returnValue != ""
}

func setPageSecurityHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
}

func renderForm(w http.ResponseWriter, status int, data pageData) {
	w.WriteHeader(status)
	_ = registrationPage.Execute(w, data)
}

func renderSuccess(w http.ResponseWriter, returnURL string) {
	w.WriteHeader(http.StatusOK)
	_ = successPage.Execute(w, pageData{ReturnURL: returnURL})
}

func renderExpired(w http.ResponseWriter) {
	w.WriteHeader(http.StatusGone)
	_ = expiredPage.Execute(w, nil)
}

var registrationPage = template.Must(template.New("registration").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Get started with Zeexco</title><style>` + pageStyles + `</style></head>
<body><main><h1>Set up your Zeexco account</h1><p>Enter your business details. Your WhatsApp identity is already securely connected to this setup link.</p>
{{if .Error}}<div class="error" role="alert">{{.Error}}</div>{{end}}
<form method="post" action="/get-started"><input type="hidden" name="token" value="{{.Token}}">
<label>First name<input required maxlength="100" autocomplete="given-name" name="first_name" value="{{.FirstName}}"></label>
<label>Last name<input required maxlength="100" autocomplete="family-name" name="last_name" value="{{.LastName}}"></label>
<label>Email (optional)<input maxlength="255" type="email" autocomplete="email" name="email" value="{{.Email}}"></label>
<label>Business name<input required maxlength="200" autocomplete="organization" name="business_name" value="{{.BusinessName}}"></label>
<button type="submit">Complete setup</button></form></main></body></html>`))

var successPage = template.Must(template.New("success").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>You're all set</title><style>` + pageStyles + `</style></head><body><main class="center">
<div class="celebrate">🎉</div><h1>You're all set</h1><p>Your Zeexco account is ready.</p><p>Let's create your first product.</p>
<a class="button" href="{{.ReturnURL}}">Continue in WhatsApp</a></main></body></html>`))

var expiredPage = template.Must(template.New("expired").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Setup link unavailable</title><style>` + pageStyles + `</style></head><body><main class="center">
<h1>This setup link is no longer available</h1><p>Return to WhatsApp and message Zeexco to start again.</p></main></body></html>`))

const pageStyles = `*{box-sizing:border-box}body{margin:0;background:#f5f7f6;color:#14211b;font:16px system-ui,sans-serif}main{max-width:480px;margin:0 auto;padding:32px 20px}h1{font-size:28px;line-height:1.15;margin:12px 0}p{line-height:1.5;color:#506158}label{display:block;font-weight:650;margin:18px 0 6px}input{width:100%;padding:14px;border:1px solid #b8c3bd;border-radius:10px;font:inherit;background:#fff}button,.button{display:block;width:100%;margin-top:24px;padding:15px;border:0;border-radius:10px;background:#126c45;color:#fff;text-align:center;text-decoration:none;font-weight:750;font:inherit;cursor:pointer}.error{padding:12px;border-radius:8px;background:#fff0f0;color:#8b2020}.center{text-align:center;padding-top:72px}.celebrate{font-size:48px}`
