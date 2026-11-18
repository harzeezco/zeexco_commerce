package controllers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/harzeezco/zeexco_commerce/internal/features/whatsapp/ingestion"
	"github.com/harzeezco/zeexco_commerce/internal/features/whatsapp/models"
)

const (
	subscribeMode             = "subscribe"
	signatureHeader           = "X-Hub-Signature-256"
	signaturePrefix           = "sha256="
	maxWebhookBodyBytes int64 = 1 << 20
)

type WebhookController struct {
	verifyToken   string
	metaAppSecret string
	ingester      WebhookIngester
	notifier      WebhookNotifier
	logger        *log.Logger
}

type WebhookIngester interface {
	AcceptWebhook(ctx context.Context, payload []byte) (ingestion.WebhookResult, error)
}

type WebhookNotifier interface{ Notify() }

type webhookEnvelope struct {
	Object json.RawMessage `json:"object"`
}

func (c *WebhookController) SetNotifier(notifier WebhookNotifier) { c.notifier = notifier }

func NewWebhookController(
	verifyToken string,
	metaAppSecret string,
	ingester WebhookIngester,
	logger *log.Logger,
) *WebhookController {
	if logger == nil {
		logger = log.Default()
	}

	return &WebhookController{
		verifyToken:   verifyToken,
		metaAppSecret: metaAppSecret,
		ingester:      ingester,
		logger:        logger,
	}
}

func (c *WebhookController) Verify(w http.ResponseWriter, r *http.Request) {
	mode, modeOK := singleQueryValue(r, "hub.mode")
	providedToken, tokenOK := singleQueryValue(r, "hub.verify_token")
	challenge, challengeOK := singleQueryValue(r, "hub.challenge")

	if !modeOK || !tokenOK || !challengeOK ||
		mode != subscribeMode || c.verifyToken == "" ||
		!tokensEqual(providedToken, c.verifyToken) {
		http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(challenge))
}

func (c *WebhookController) Receive(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxWebhookBodyBytes))
	if err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			c.logger.Printf("whatsapp webhook rejected outcome=request_too_large")
			http.Error(w, http.StatusText(http.StatusRequestEntityTooLarge), http.StatusRequestEntityTooLarge)
			return
		}

		c.logger.Printf("whatsapp webhook rejected outcome=bad_request reason=body_read_failed")
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	if !c.hasValidSignature(r, body) {
		c.logger.Printf("whatsapp webhook rejected outcome=forbidden reason=invalid_signature")
		http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		return
	}

	_, ok := parseWebhookObject(body)
	if !ok {
		c.logger.Printf("whatsapp webhook rejected outcome=bad_request reason=invalid_envelope")
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	if c.ingester == nil {
		c.logger.Printf("whatsapp webhook rejected outcome=server_error reason=ingestion_unavailable")
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	webhookResult, err := c.ingester.AcceptWebhook(r.Context(), body)
	if err != nil {
		var invalid *ingestion.InvalidWebhookError
		if errors.As(err, &invalid) {
			c.logger.Printf("whatsapp webhook rejected outcome=bad_request reason=message_parse_failed")
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}
		c.logger.Printf("whatsapp webhook rejected outcome=server_error reason=ingestion_failed")
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	if webhookResult.Object != "whatsapp_business_account" {
		c.logger.Printf("whatsapp webhook accepted object=unsupported outcome=ok event_id=%s", webhookResult.EventID)
		writeWebhookAccepted(w)
		return
	}

	c.logger.Printf(
		"whatsapp webhook accepted channel=whatsapp object=whatsapp_business_account outcome=durable event_id=%s",
		webhookResult.EventID,
	)
	if c.notifier != nil {
		c.notifier.Notify()
	}

	writeWebhookAccepted(w)
}

func ingestionCounts(results []ingestion.Result) (
	stored int,
	duplicate int,
	registered int,
	unknown int,
) {
	for _, result := range results {
		switch result.Status {
		case ingestion.StatusNew:
			stored++
			if result.Identity.Registered {
				registered++
			} else {
				unknown++
			}
		case ingestion.StatusDuplicate:
			duplicate++
		}
	}

	return stored, duplicate, registered, unknown
}

func writeWebhookAccepted(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

func messageTypeCounts(messages []models.IncomingMessage) (
	text int,
	interactive int,
	button int,
	unsupported int,
) {
	for _, message := range messages {
		if !message.Supported {
			unsupported++
			continue
		}

		switch message.Type {
		case "text":
			text++
		case "interactive":
			interactive++
		case "button":
			button++
		default:
			unsupported++
		}
	}

	return text, interactive, button, unsupported
}

func (c *WebhookController) hasValidSignature(r *http.Request, body []byte) bool {
	if c.metaAppSecret == "" {
		return false
	}

	headerValues := r.Header.Values(signatureHeader)
	if len(headerValues) != 1 || !strings.HasPrefix(headerValues[0], signaturePrefix) {
		return false
	}

	encodedSignature := strings.TrimPrefix(headerValues[0], signaturePrefix)
	if len(encodedSignature) != sha256.Size*2 {
		return false
	}

	receivedSignature, err := hex.DecodeString(encodedSignature)
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, []byte(c.metaAppSecret))
	_, _ = mac.Write(body)
	expectedSignature := mac.Sum(nil)

	return hmac.Equal(receivedSignature, expectedSignature)
}

func parseWebhookObject(body []byte) (string, bool) {
	var envelope webhookEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil || envelope.Object == nil {
		return "", false
	}

	var object string
	if err := json.Unmarshal(envelope.Object, &object); err != nil || object == "" {
		return "", false
	}

	return object, true
}

func singleQueryValue(r *http.Request, key string) (string, bool) {
	values, ok := r.URL.Query()[key]
	if !ok || len(values) != 1 || values[0] == "" {
		return "", false
	}

	return values[0], true
}

func tokensEqual(provided, expected string) bool {
	providedHash := sha256.Sum256([]byte(provided))
	expectedHash := sha256.Sum256([]byte(expected))

	return subtle.ConstantTimeCompare(providedHash[:], expectedHash[:]) == 1
}
