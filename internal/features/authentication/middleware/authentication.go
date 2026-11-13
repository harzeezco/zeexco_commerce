package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/harzeezco/zeexco_commerce/internal/features/authentication/models"
)

type AccessTokenVerifier interface {
	Verify(raw string) (models.Principal, error)
}

type principalContextKey struct{}

type Authentication struct {
	verifier AccessTokenVerifier
}

func NewAuthentication(verifier AccessTokenVerifier) *Authentication {
	return &Authentication{verifier: verifier}
}

func (m *Authentication) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawToken, ok := bearerToken(r)
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
