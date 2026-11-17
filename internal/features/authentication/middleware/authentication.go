package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/harzeezco/zeexco_commerce/internal/features/authentication/models"
	"github.com/harzeezco/zeexco_commerce/internal/platform/httpapi"
)

type AccessTokenVerifier interface {
	Verify(raw string) (models.Principal, error)
}

type principalContextKey struct{}

type Authentication struct {
	verifier AccessTokenVerifier
	cookie   CookieAuthenticationConfig
}

type CookieAuthenticationConfig struct {
	Name          string
	AllowedOrigin string
}

func NewAuthentication(verifier AccessTokenVerifier, config ...CookieAuthenticationConfig) *Authentication {
	cookie := CookieAuthenticationConfig{Name: "zeexco_access"}
	if len(config) > 0 {
		cookie = config[0]
	}
	return &Authentication{verifier: verifier, cookie: cookie}
}

func (m *Authentication) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Pragma", "no-cache")
		rawToken, ok := bearerToken(r)
		cookieCount := 0
		for _, cookie := range r.Cookies() {
			if cookie.Name == m.cookie.Name {
				cookieCount++
				rawToken, ok = cookie.Value, cookie.Value != ""
			}
		}
		if cookieCount > 1 {
			writeUnauthorized(w)
			return
		}
		if !ok || m.verifier == nil {
			writeUnauthorized(w)
			return
		}
		principal, err := m.verifier.Verify(rawToken)
		if err != nil || !models.IsUUID(principal.UserID) {
			writeUnauthorized(w)
			return
		}
		ctx := context.WithValue(r.Context(), principalContextKey{}, principal)
		if cookieCount == 1 && r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
			httpapi.RequireOrigin(m.cookie.AllowedOrigin, next).ServeHTTP(w, r.WithContext(ctx))
			return
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func PrincipalFromContext(ctx context.Context) (models.Principal, bool) {
	principal, ok := ctx.Value(principalContextKey{}).(models.Principal)
	return principal, ok && models.IsUUID(principal.UserID)
}

func bearerToken(r *http.Request) (string, bool) {
	values := r.Header.Values("Authorization")
	if len(values) != 1 {
		return "", false
	}
	parts := strings.Fields(values[0])
	if len(parts) != 2 || parts[0] != "Bearer" || parts[1] == "" {
		return "", false
	}
	return parts[1], true
}

func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("WWW-Authenticate", "Bearer")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{
			"code": "UNAUTHORIZED", "message": "authentication is required",
		},
	})
}
