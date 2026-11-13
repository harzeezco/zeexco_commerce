package models

import (
	"encoding/json"
	"time"
)

type WebhookEnvelope struct {
	Object string         `json:"object"`
	Entry  []WebhookEntry `json:"entry"`
}

type WebhookEntry struct {
	ID      string          `json:"id"`
	Changes []WebhookChange `json:"changes"`
}

type WebhookChange struct {
	Field string        `json:"field"`
	Value *WebhookValue `json:"value"`
}

type WebhookValue struct {
	MessagingProduct string            `json:"messaging_product"`
	Metadata         WebhookMetadata   `json:"metadata"`
	Contacts         []WebhookContact  `json:"contacts"`
	Messages         []WebhookMessage  `json:"messages"`
	Statuses         []json.RawMessage `json:"statuses"`
}

type WebhookMetadata struct {
	DisplayPhoneNumber string `json:"display_phone_number"`
	PhoneNumberID      string `json:"phone_number_id"`
}

type WebhookContact struct {
	Profile    WebhookContactProfile `json:"profile"`
	WhatsAppID string                `json:"wa_id"`
}

type WebhookContactProfile struct {
	Name string `json:"name"`
}

type WebhookMessage struct {
	ID          string              `json:"id"`
	From        string              `json:"from"`
	Timestamp   string              `json:"timestamp"`
	Type        string              `json:"type"`
	Text        *WebhookText        `json:"text,omitempty"`
	Interactive *WebhookInteractive `json:"interactive,omitempty"`
	Button      *WebhookButton      `json:"button,omitempty"`
}

type WebhookText struct {
	Body string `json:"body"`
}

type WebhookInteractive struct {
	Type        string        `json:"type"`
	ButtonReply *WebhookReply `json:"button_reply,omitempty"`
	ListReply   *WebhookReply `json:"list_reply,omitempty"`
}

type WebhookReply struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type WebhookButton struct {
	Payload string `json:"payload"`
	Text    string `json:"text"`
}

type IncomingMessage struct {
	ID                    string
	SenderWhatsAppID      string
	BusinessPhoneNumberID string
	Timestamp             *time.Time
	Type                  string
	Supported             bool
	Text                  string
	InteractiveButton     *IncomingReply
	InteractiveList       *IncomingReply
	TemplateButton        *IncomingButton
}

// WebhookEvent is the immutable provider delivery evidence and its processing
// lifecycle. Payload contains the original verified request JSON bytes.
type WebhookEvent struct {
	ID                  string
	Provider            string
	ProviderAccountID   string
	PhoneNumberID       string
	EventType           string
	Payload             json.RawMessage
	ProcessingStatus    string
	ReceivedAt          time.Time
	ProcessingStartedAt *time.Time
	ProcessedAt         *time.Time
	AttemptCount        int
	LastErrorCode       string
	LastErrorMessage    string
}

type IncomingReply struct {
	ID    string
	Title string
}

type IncomingButton struct {
	Payload string
	Text    string
}
