package flows

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
)

const (
	maxFlowBodyBytes    int64 = 64 << 10
	flowSignatureHeader       = "X-Hub-Signature-256"
	flowSignaturePrefix       = "sha256="
)

type Controller struct {
	protocol      *Protocol
	actions       *ActionHandler
	metaAppSecret string
	logger        *log.Logger
}

func NewController(protocol *Protocol, actions *ActionHandler, metaAppSecret string, logger *log.Logger) (*Controller, error) {
	if protocol == nil || actions == nil || metaAppSecret == "" {
		return nil, errors.New("Flow protocol, action handler, and Meta app secret are required")
	}
	if logger == nil {
		logger = log.Default()
	}
	return &Controller{protocol: protocol, actions: actions, metaAppSecret: metaAppSecret, logger: logger}, nil
}

func (c *Controller) Exchange(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxFlowBodyBytes))
	if err != nil {
		c.reject(w, http.StatusBadRequest, "body_read_failed")
		return
	}
	if !validSignature(r.Header.Values(flowSignatureHeader), body, c.metaAppSecret) {
		c.reject(w, 432, "invalid_signature")
		return
	}
	var envelope EncryptedRequest
	if err := decodeStrict(body, &envelope); err != nil {
		c.reject(w, http.StatusBadRequest, "invalid_envelope")
		return
	}
	decrypted, err := c.protocol.Decrypt(envelope)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, ErrDecrypt) {
			status = 421
		}
		c.reject(w, status, "decryption_failed")
		return
	}

	var actionOnly struct {
		Action string `json:"action"`
	}
	_ = json.Unmarshal(decrypted.Payload, &actionOnly)
	response, err := c.actions.Handle(r.Context(), decrypted.Payload)
	status := http.StatusOK
	if err != nil {
		var domain *DomainError
		switch {
		case errors.As(err, &domain):
			response = DomainErrorResponse(domain)
			status = domain.Status
		case errors.Is(err, ErrUnsupportedAction):
			response = map[string]string{"error_msg": "Unsupported Flow action."}
			status = http.StatusBadRequest
		case errors.Is(err, ErrInvalidPayload):
			response = map[string]string{"error_msg": "Invalid Flow request."}
			status = http.StatusBadRequest
		default:
			c.reject(w, http.StatusInternalServerError, "action_failed")
			return
		}
	}
	encrypted, err := c.protocol.Encrypt(response, decrypted.AESKey, decrypted.InitialVector)
	if err != nil {
		c.reject(w, http.StatusInternalServerError, "encryption_failed")
		return
	}
	c.logger.Printf("whatsapp flow channel=whatsapp operation=flow_data_exchange action=%s outcome=ok", safeAction(actionOnly.Action))
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(encrypted))
}

func (c *Controller) reject(w http.ResponseWriter, status int, reason string) {
	c.logger.Printf("whatsapp flow channel=whatsapp operation=flow_data_exchange outcome=rejected reason=%s", reason)
	w.Header().Set("Cache-Control", "no-store")
	http.Error(w, http.StatusText(status), status)
}

func validSignature(values []string, body []byte, secret string) bool {
	if secret == "" || len(values) != 1 || !strings.HasPrefix(values[0], flowSignaturePrefix) {
		return false
	}
	received, err := hex.DecodeString(strings.TrimPrefix(values[0], flowSignaturePrefix))
	if err != nil || len(received) != sha256.Size {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return hmac.Equal(received, mac.Sum(nil))
}

func safeAction(action string) string {
	switch action {
	case "ping", "INIT", "data_exchange":
		return action
	default:
		return "unsupported"
	}
}
