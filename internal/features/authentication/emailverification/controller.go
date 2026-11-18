package emailverification

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

const maxVerifyBodyBytes int64 = 8 << 10

type Verifier interface {
	Verify(context.Context, string) error
}

type Controller struct{ service Verifier }

func NewController(service Verifier) *Controller { return &Controller{service: service} }

func (c *Controller) Verify(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var request struct {
		Token string `json:"token"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxVerifyBodyBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeVerificationError(w, http.StatusBadRequest, "INVALID_JSON", "request body must be a single valid JSON object")
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeVerificationError(w, http.StatusBadRequest, "INVALID_JSON", "request body must be a single valid JSON object")
		return
	}
	var result VerificationResult
	var err error
	if service, ok := c.service.(interface {
		VerifyWithResult(context.Context, string) (VerificationResult, error)
	}); ok {
		result, err = service.VerifyWithResult(r.Context(), request.Token)
	} else {
		err = c.service.Verify(r.Context(), request.Token)
	}
	switch {
	case err == nil:
		if result.ContinueInWhatsApp {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(struct {
				StatusCode  int    `json:"status_code"`
				Message     string `json:"message"`
				NextStep    string `json:"next_step"`
				RedirectURL string `json:"redirect_url,omitempty"`
			}{http.StatusOK, "Account activated successfully. Continue in WhatsApp.", "CONTINUE_IN_WHATSAPP", result.RedirectURL})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, ErrInvalidToken):
		writeVerificationError(w, http.StatusBadRequest, "EMAIL_VERIFICATION_INVALID", "email verification link is invalid")
	case errors.Is(err, ErrVerificationExpired):
		writeVerificationError(w, http.StatusGone, "EMAIL_VERIFICATION_EXPIRED", "email verification link has expired")
	case errors.Is(err, ErrVerificationConsumed):
		writeVerificationError(w, http.StatusConflict, "EMAIL_VERIFICATION_CONSUMED", "email verification link has already been used")
	default:
		writeVerificationError(w, http.StatusInternalServerError, "EMAIL_VERIFICATION_FAILED", "email verification could not be completed")
	}
}

func writeVerificationError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{
		"code": code, "message": message,
	}})
}
