package parser

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/harzeezco/zeexco_commerce/internal/features/whatsapp/models"
)

const (
	SupportedWebhookObject = "whatsapp_business_account"
	messagesField          = "messages"
	messageTypeText        = "text"
	messageTypeInteractive = "interactive"
	messageTypeButton      = "button"
	interactiveButtonReply = "button_reply"
	interactiveListReply   = "list_reply"
)

func ParseMessages(payload []byte) ([]models.IncomingMessage, error) {
	var envelope models.WebhookEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return nil, fmt.Errorf("decode WhatsApp webhook envelope: %w", err)
	}

	if envelope.Object != SupportedWebhookObject {
		return nil, fmt.Errorf("unexpected WhatsApp webhook object")
	}

	messages := make([]models.IncomingMessage, 0)
	for entryIndex, entry := range envelope.Entry {
		for changeIndex, change := range entry.Changes {
			if strings.TrimSpace(change.Field) == "" {
				return nil, fmt.Errorf("entry %d change %d: change field is required", entryIndex, changeIndex)
			}

			if change.Field != messagesField {
				continue
			}

			if change.Value == nil {
				return nil, fmt.Errorf("entry %d change %d: messages value is required", entryIndex, changeIndex)
			}
			if len(change.Value.Messages) > 0 &&
				strings.TrimSpace(change.Value.Metadata.PhoneNumberID) == "" {
				return nil, fmt.Errorf(
					"entry %d change %d: business phone number ID is required for messages",
					entryIndex,
					changeIndex,
				)
			}

			for messageIndex, message := range change.Value.Messages {
				normalized, err := normalizeMessage(message, change.Value.Metadata.PhoneNumberID)
				if err != nil {
					return nil, fmt.Errorf(
						"entry %d change %d message %d: %w",
						entryIndex,
						changeIndex,
						messageIndex,
						err,
					)
				}

				messages = append(messages, normalized)
			}
		}
	}

	return messages, nil
}

func normalizeMessage(
	message models.WebhookMessage,
	businessPhoneNumberID string,
) (models.IncomingMessage, error) {
	if strings.TrimSpace(message.ID) == "" {
		return models.IncomingMessage{}, fmt.Errorf("message ID is required")
	}

	if strings.TrimSpace(message.From) == "" {
		return models.IncomingMessage{}, fmt.Errorf("sender WhatsApp ID is required")
	}

	if strings.TrimSpace(message.Type) == "" {
		return models.IncomingMessage{}, fmt.Errorf("message type is required")
	}

	timestamp, err := parseTimestamp(message.Timestamp)
	if err != nil {
		return models.IncomingMessage{}, err
	}

	normalized := models.IncomingMessage{
		ID:                    message.ID,
		SenderWhatsAppID:      message.From,
		BusinessPhoneNumberID: businessPhoneNumberID,
		Timestamp:             timestamp,
		Type:                  message.Type,
	}

	switch message.Type {
	case messageTypeText:
		if message.Text == nil || strings.TrimSpace(message.Text.Body) == "" {
			return models.IncomingMessage{}, fmt.Errorf("text body is required")
		}

		normalized.Supported = true
		normalized.Text = message.Text.Body
	case messageTypeInteractive:
		if message.Interactive == nil || strings.TrimSpace(message.Interactive.Type) == "" {
			return models.IncomingMessage{}, fmt.Errorf("interactive reply type is required")
		}

		switch message.Interactive.Type {
		case interactiveButtonReply:
			reply, err := normalizeReply(message.Interactive.ButtonReply, "interactive button reply")
			if err != nil {
				return models.IncomingMessage{}, err
			}

			normalized.Supported = true
			normalized.InteractiveButton = reply
		case interactiveListReply:
			reply, err := normalizeReply(message.Interactive.ListReply, "interactive list reply")
			if err != nil {
				return models.IncomingMessage{}, err
			}

			normalized.Supported = true
			normalized.InteractiveList = reply
		}
	case messageTypeButton:
		if message.Button == nil ||
			strings.TrimSpace(message.Button.Payload) == "" ||
			strings.TrimSpace(message.Button.Text) == "" {
			return models.IncomingMessage{}, fmt.Errorf("template button payload and text are required")
		}

		normalized.Supported = true
		normalized.TemplateButton = &models.IncomingButton{
			Payload: message.Button.Payload,
			Text:    message.Button.Text,
		}
	}

	return normalized, nil
}

func normalizeReply(reply *models.WebhookReply, name string) (*models.IncomingReply, error) {
	if reply == nil || strings.TrimSpace(reply.ID) == "" || strings.TrimSpace(reply.Title) == "" {
		return nil, fmt.Errorf("%s ID and title are required", name)
	}

	return &models.IncomingReply{
		ID:    reply.ID,
		Title: reply.Title,
	}, nil
}

func parseTimestamp(value string) (*time.Time, error) {
	if value == "" {
		return nil, nil
	}

	seconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil || seconds < 0 {
		return nil, fmt.Errorf("message timestamp must be non-negative Unix seconds")
	}

	timestamp := time.Unix(seconds, 0).UTC()
	return &timestamp, nil
}
