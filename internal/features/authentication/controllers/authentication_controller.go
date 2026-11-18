package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	authmiddleware "github.com/harzeezco/zeexco_commerce/internal/features/authentication/middleware"
	"github.com/harzeezco/zeexco_commerce/internal/features/authentication/models"
	"github.com/harzeezco/zeexco_commerce/internal/features/authentication/schemas"
	"github.com/harzeezco/zeexco_commerce/internal/features/authentication/services"
)

type AuthenticationService interface {
	Login(context.Context, services.LoginCommand) (services.AuthenticationResult, error)
	Refresh(context.Context, string) (services.AuthenticationResult, error)
	Logout(context.Context, string) error
	Me(context.Context, models.Principal) (models.UserContext, error)
}

type SessionCookieConfig struct {
	AccessName string
	Name       string
	Path       string
	Domain     string
	Secure     bool
	SameSite   http.SameSite
	Lifetime   time.Duration
}

type AuthenticationController struct {
	service AuthenticationService
	cookie  SessionCookieConfig
}

func NewAuthenticationController(
	service AuthenticationService,
	cookie SessionCookieConfig,
) *AuthenticationController {
	return &AuthenticationController{service: service, cookie: cookie}
}

func (c *AuthenticationController) Login(w http.ResponseWriter, r *http.Request) {
	var request schemas.LoginRequest
	if err := decodeAuthenticationJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "request body must be a single valid JSON object", "")
		return
	}

	result, err := c.service.Login(r.Context(), services.LoginCommand{
		Email: request.Email, Password: request.Password,
	})

	if errors.Is(err, services.ErrInvalidCredentials) {
		writeError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "email or password is invalid", "")
		return
	}

	if errors.Is(err, services.ErrEmailVerificationRequired) {
		writeError(w, http.StatusForbidden, "EMAIL_VERIFICATION_REQUIRED", "Account is pending verification. Please check your email to activate your account.", "")
		return
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, "AUTHENTICATION_FAILED", "authentication could not be completed", "")
		return
	}

	c.setSessionCookies(w, result)
	setAuthenticationCacheHeaders(w)
	writeJSON(w, http.StatusOK, schemas.LoginResponse{
		StatusCode: http.StatusOK,
		Message:    "Login successful. Welcome back",
	})
}

func (c *AuthenticationController) Refresh(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(c.cookie.Name)
	if err != nil || cookie.Value == "" {
		c.clearSessionCookies(w)
		writeError(w, http.StatusUnauthorized, "INVALID_REFRESH_SESSION", "refresh session is invalid", "")
		return
	}

	result, err := c.service.Refresh(r.Context(), cookie.Value)
	if errors.Is(err, services.ErrInvalidRefresh) {
		c.clearSessionCookies(w)
		writeError(w, http.StatusUnauthorized, "INVALID_REFRESH_SESSION", "refresh session is invalid", "")
		return
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, "REFRESH_FAILED", "session refresh could not be completed", "")
		return
	}
	c.setSessionCookies(w, result)
	setAuthenticationCacheHeaders(w)
	writeJSON(w, http.StatusOK, schemas.LoginResponse{
		StatusCode: http.StatusOK, Message: "Session refreshed successfully.",
	})
}

func (c *AuthenticationController) Logout(w http.ResponseWriter, r *http.Request) {
	rawToken := ""
	if cookie, err := r.Cookie(c.cookie.Name); err == nil {
		rawToken = cookie.Value
	}
	if err := c.service.Logout(r.Context(), rawToken); err != nil {
		writeError(w, http.StatusInternalServerError, "LOGOUT_FAILED", "logout could not be completed", "")
		return
	}
	c.clearSessionCookies(w)
	w.WriteHeader(http.StatusNoContent)
}

func (c *AuthenticationController) Me(w http.ResponseWriter, r *http.Request) {
	setAuthenticationCacheHeaders(w)
	principal, ok := authmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "authentication is required", "")
		return
	}
	userContext, err := c.service.Me(r.Context(), principal)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "CURRENT_USER_FAILED", "current user could not be loaded", "")
		return
	}
	writeJSON(w, http.StatusOK, meResponse(userContext))
}

// Session loads current authorization context from PostgreSQL rather than
// trusting roles or account status captured when the access JWT was issued.
func (c *AuthenticationController) Session(w http.ResponseWriter, r *http.Request) {
	setAuthenticationCacheHeaders(w)
	principal, ok := authmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", "")
		return
	}
	userContext, err := c.service.Me(r.Context(), principal)
	if errors.Is(err, models.ErrCredentialsNotFound) {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Session is no longer valid. Please log in again.", "")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SESSION_LOAD_FAILED", "Session could not be loaded", "")
		return
	}
	if userContext.User.Status != models.UserStatusActive {
		writeError(w, http.StatusForbidden, "ACCOUNT_NOT_ACTIVE", "Account is not active", "")
		return
	}
	writeJSON(w, http.StatusOK, schemas.SessionResponse{
		StatusCode: http.StatusOK, Message: "Session retrieved successfully.",
		User: registeredUser(userContext.User), Businesses: businessResponses(userContext),
	})
}

func (c *AuthenticationController) setSessionCookies(w http.ResponseWriter, result services.AuthenticationResult) {
	http.SetCookie(w, &http.Cookie{
		Name: c.cookie.AccessName, Value: result.AccessToken, Path: "/",
		Domain: c.cookie.Domain, MaxAge: int(result.ExpiresIn),
		Expires:  time.Now().UTC().Add(time.Duration(result.ExpiresIn) * time.Second),
		HttpOnly: true, Secure: c.cookie.Secure, SameSite: c.cookie.SameSite,
	})
	http.SetCookie(w, &http.Cookie{
		Name: c.cookie.Name, Value: result.RefreshToken, Path: c.cookie.Path,
		Domain: c.cookie.Domain, MaxAge: int(c.cookie.Lifetime.Seconds()),
		Expires:  time.Now().UTC().Add(c.cookie.Lifetime),
		HttpOnly: true, Secure: c.cookie.Secure, SameSite: c.cookie.SameSite,
	})
}

func (c *AuthenticationController) clearSessionCookies(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: c.cookie.AccessName, Value: "", Path: "/", Domain: c.cookie.Domain,
		MaxAge: -1, Expires: time.Unix(1, 0).UTC(), HttpOnly: true,
		Secure: c.cookie.Secure, SameSite: c.cookie.SameSite,
	})
	http.SetCookie(w, &http.Cookie{
		Name: c.cookie.Name, Value: "", Path: c.cookie.Path, Domain: c.cookie.Domain,
		MaxAge: -1, Expires: time.Unix(1, 0).UTC(), HttpOnly: true,
		Secure: c.cookie.Secure, SameSite: c.cookie.SameSite,
	})
}

func setAuthenticationCacheHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
}

func decodeAuthenticationJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRegistrationBodyBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request contains multiple JSON values")
	}
	return nil
}

func meResponse(userContext models.UserContext) schemas.MeResponse {
	return schemas.MeResponse{
		User: registeredUser(userContext.User), Businesses: businessResponses(userContext),
	}
}

func registeredUser(user models.User) schemas.RegisteredUser {
	return schemas.RegisteredUser{
		ID: user.ID, FirstName: user.FirstName, LastName: user.LastName,
		Email: user.Email, Status: string(user.Status),
	}
}

func businessResponses(userContext models.UserContext) []schemas.AuthenticatedBusiness {
	result := make([]schemas.AuthenticatedBusiness, 0, len(userContext.Businesses))
	for _, item := range userContext.Businesses {
		result = append(result, schemas.AuthenticatedBusiness{
			ID: item.Business.ID, Name: item.Business.Name, Status: string(item.Business.Status),
			MembershipRole: string(item.Membership.Role),
		})
	}
	return result
}
