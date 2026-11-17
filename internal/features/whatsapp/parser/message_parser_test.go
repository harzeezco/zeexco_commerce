package parser

import (
	"testing"
	"time"
)

func TestParseMessagesParsesSingleTextMessage(t *testing.T) {
	payload := []byte(`{
        "object": "whatsapp_business_account",
        "entry": [{
            "id": "business-account-1",
            "changes": [{
                "field": "messages",
                "value": {
                    "messaging_product": "whatsapp",
                    "metadata": {
                        "display_phone_number": "15550000000",
                        "phone_number_id": "business-phone-id-1"
                    },
                    "contacts": [{
                        "profile": {"name": "Sanitized Customer"},
                        "wa_id": "sender-wa-id-1"
                    }],
                    "messages": [{
                        "from": "sender-wa-id-1",
                        "id": "message-id-1",
                        "timestamp": "1700000000",
                        "type": "text",
                        "text": {"body": "Hello Zeexco"}
                    }]
                }
            }]
        }]
    }`)

	messages, err := ParseMessages(payload)
	if err != nil {
		t.Fatalf("ParseMessages() error = %v", err)
	}

	if len(messages) != 1 {
		t.Fatalf("message count = %d, want 1", len(messages))
	}

	message := messages[0]
	if message.ID != "message-id-1" ||
		message.SenderWhatsAppID != "sender-wa-id-1" ||
		message.BusinessPhoneNumberID != "business-phone-id-1" ||
		message.Type != "text" ||
		!message.Supported ||
		message.Text != "Hello Zeexco" {
		t.Fatalf("parsed message = %#v", message)
	}

	wantTimestamp := time.Unix(1700000000, 0).UTC()
	if message.Timestamp == nil || !message.Timestamp.Equal(wantTimestamp) {
		t.Fatalf("timestamp = %v, want %v", message.Timestamp, wantTimestamp)
	}
}

func TestParseMessagesParsesMultipleMessages(t *testing.T) {
	payload := payloadWithMessages(`
        {
            "from":"sender-1",
            "id":"message-1",
            "timestamp":"1700000001",
            "type":"text",
            "text":{"body":"First"}
        },
        {
            "from":"sender-2",
            "id":"message-2",
            "timestamp":"1700000002",
            "type":"image"
        }
    `)

	messages, err := ParseMessages(payload)
	if err != nil {
		t.Fatalf("ParseMessages() error = %v", err)
	}

	if len(messages) != 2 {
		t.Fatalf("message count = %d, want 2", len(messages))
	}

	if messages[0].ID != "message-1" || messages[1].ID != "message-2" {
		t.Fatalf("message order was not preserved: %#v", messages)
	}

	if !messages[0].Supported || messages[1].Supported || messages[1].Type != "image" {
		t.Fatalf("supported states = %#v", messages)
	}
}

func TestParseMessagesTraversesMultipleEntriesAndChanges(t *testing.T) {
	payload := []byte(`{
        "object":"whatsapp_business_account",
        "entry":[
            {
                "id":"account-1",
                "changes":[
                    {"field":"account_update","value":{}},
                    {
                        "field":"messages",
                        "value":{
                            "metadata":{"phone_number_id":"business-phone-1"},
                            "messages":[{
                                "from":"sender-1",
                                "id":"message-1",
                                "timestamp":"1700000001",
                                "type":"text",
                                "text":{"body":"First entry"}
                            }]
                        }
                    }
                ]
            },
            {
                "id":"account-2",
                "changes":[
                    {
                        "field":"messages",
                        "value":{
                            "metadata":{"phone_number_id":"business-phone-2"},
                            "messages":[{
                                "from":"sender-2",
                                "id":"message-2",
                                "timestamp":"1700000002",
                                "type":"button",
                                "button":{"payload":"confirm-order","text":"Confirm"}
                            }]
                        }
                    },
                    {
                        "field":"messages",
                        "value":{"statuses":[{"id":"status-only"}]}
                    }
                ]
            }
        ]
    }`)

	messages, err := ParseMessages(payload)
	if err != nil {
		t.Fatalf("ParseMessages() error = %v", err)
	}

	if len(messages) != 2 {
		t.Fatalf("message count = %d, want 2", len(messages))
	}

	if messages[0].BusinessPhoneNumberID != "business-phone-1" ||
		messages[1].BusinessPhoneNumberID != "business-phone-2" {
		t.Fatalf("business phone IDs were not kept with their changes: %#v", messages)
	}
}

func TestParseMessagesParsesReplyTypes(t *testing.T) {
	tests := []struct {
		name    string
		message string
		check   func(*testing.T, parsedMessage)
	}{
		{
			name: "interactive button reply",
			message: `{
                "from":"sender-1","id":"message-1","timestamp":"1700000001",
                "type":"interactive",
                "interactive":{
                    "type":"button_reply",
                    "button_reply":{"id":"choose-blue","title":"Blue"}
                }
            }`,
			check: func(t *testing.T, message parsedMessage) {
				t.Helper()
				if message.interactiveButtonID != "choose-blue" || message.interactiveButtonTitle != "Blue" {
					t.Fatalf("button reply = %#v", message)
				}
			},
		},
		{
			name: "interactive list reply",
			message: `{
                "from":"sender-1","id":"message-1","timestamp":"1700000001",
                "type":"interactive",
                "interactive":{
                    "type":"list_reply",
                    "list_reply":{"id":"product-42","title":"Product 42"}
                }
            }`,
			check: func(t *testing.T, message parsedMessage) {
				t.Helper()
				if message.interactiveListID != "product-42" || message.interactiveListTitle != "Product 42" {
					t.Fatalf("list reply = %#v", message)
				}
			},
		},
		{
			name: "template quick reply button",
			message: `{
                "from":"sender-1","id":"message-1","timestamp":"1700000001",
                "type":"button",
                "button":{"payload":"continue-registration","text":"Continue"}
            }`,
			check: func(t *testing.T, message parsedMessage) {
				t.Helper()
				if message.buttonPayload != "continue-registration" || message.buttonText != "Continue" {
					t.Fatalf("template button = %#v", message)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messages, err := ParseMessages(payloadWithMessages(tt.message))
			if err != nil {
				t.Fatalf("ParseMessages() error = %v", err)
			}

			if len(messages) != 1 || !messages[0].Supported {
				t.Fatalf("messages = %#v", messages)
			}

			actual := parsedMessage{}
			if messages[0].InteractiveButton != nil {
				actual.interactiveButtonID = messages[0].InteractiveButton.ID
				actual.interactiveButtonTitle = messages[0].InteractiveButton.Title
			}
			if messages[0].InteractiveList != nil {
				actual.interactiveListID = messages[0].InteractiveList.ID
				actual.interactiveListTitle = messages[0].InteractiveList.Title
			}
			if messages[0].TemplateButton != nil {
				actual.buttonPayload = messages[0].TemplateButton.Payload
				actual.buttonText = messages[0].TemplateButton.Text
			}

			tt.check(t, actual)
		})
	}
}

func TestParseMessagesAcceptsStatusOnlyAndEmptyEntries(t *testing.T) {
	tests := []struct {
		name    string
		payload []byte
	}{
		{
			name:    "empty entries",
			payload: []byte(`{"object":"whatsapp_business_account","entry":[]}`),
		},
		{
			name: "empty changes",
			payload: []byte(`{
                "object":"whatsapp_business_account",
                "entry":[{"id":"account-1","changes":[]}]
            }`),
		},
		{
			name: "status only",
			payload: []byte(`{
                "object":"whatsapp_business_account",
                "entry":[{"changes":[{
                    "field":"messages",
                    "value":{"statuses":[{"id":"status-1","status":"delivered"}]}
                }]}]
            }`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messages, err := ParseMessages(tt.payload)
			if err != nil {
				t.Fatalf("ParseMessages() error = %v", err)
			}
			if len(messages) != 0 {
				t.Fatalf("message count = %d, want 0", len(messages))
			}
		})
	}
}

func TestParseMessagesRejectsMalformedMessages(t *testing.T) {
	tests := []struct {
		name    string
		message string
	}{
		{
			name:    "missing message ID",
			message: `{"from":"sender-1","type":"text","text":{"body":"Hello"}}`,
		},
		{
			name:    "missing sender",
			message: `{"id":"message-1","type":"text","text":{"body":"Hello"}}`,
		},
		{
			name:    "missing message type",
			message: `{"from":"sender-1","id":"message-1"}`,
		},
		{
			name:    "malformed timestamp",
			message: `{"from":"sender-1","id":"message-1","timestamp":"not-a-timestamp","type":"image"}`,
		},
		{
			name:    "missing text content",
			message: `{"from":"sender-1","id":"message-1","type":"text"}`,
		},
		{
			name:    "empty text content",
			message: `{"from":"sender-1","id":"message-1","type":"text","text":{"body":" "}}`,
		},
		{
			name:    "missing interactive type",
			message: `{"from":"sender-1","id":"message-1","type":"interactive","interactive":{}}`,
		},
		{
			name:    "missing button reply content",
			message: `{"from":"sender-1","id":"message-1","type":"interactive","interactive":{"type":"button_reply","button_reply":{"id":"","title":"Choose"}}}`,
		},
		{
			name:    "missing list reply content",
			message: `{"from":"sender-1","id":"message-1","type":"interactive","interactive":{"type":"list_reply"}}`,
		},
		{
			name:    "missing template button content",
			message: `{"from":"sender-1","id":"message-1","type":"button","button":{"payload":"","text":"Continue"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseMessages(payloadWithMessages(tt.message)); err == nil {
				t.Fatal("ParseMessages() error = nil, want controlled parsing error")
			}
		})
	}
}

func TestParseMessagesRejectsInvalidWebhookStructure(t *testing.T) {
	tests := [][]byte{
		[]byte(`{"object":`),
		[]byte(`[]`),
		[]byte(`{"object":"whatsapp_business_account","entry":"invalid"}`),
		[]byte(`{"object":"unsupported_object","entry":[]}`),
		[]byte(`{"object":"whatsapp_business_account","entry":[{"changes":[{"value":{}}]}]}`),
		[]byte(`{"object":"whatsapp_business_account","entry":[{"changes":[{"field":"messages"}]}]}`),
		payloadWithMessagesWithoutBusinessPhoneID(`{
            "from":"sender-1","id":"message-1","type":"text","text":{"body":"Hello"}
        }`),
	}

	for _, payload := range tests {
		if _, err := ParseMessages(payload); err == nil {
			t.Fatalf("ParseMessages(%q) error = nil, want error", payload)
		}
	}
}

type parsedMessage struct {
	interactiveButtonID    string
	interactiveButtonTitle string
	interactiveListID      string
	interactiveListTitle   string
	buttonPayload          string
	buttonText             string
}

func payloadWithMessages(messages string) []byte {
	return []byte(`{
        "object":"whatsapp_business_account",
        "entry":[{
            "id":"business-account-1",
            "changes":[{
                "field":"messages",
                "value":{
                    "metadata":{"phone_number_id":"business-phone-id-1"},
                    "messages":[` + messages + `]
                }
            }]
        }]
    }`)
}

func payloadWithMessagesWithoutBusinessPhoneID(messages string) []byte {
	return []byte(`{
        "object":"whatsapp_business_account",
        "entry":[{"changes":[{
            "field":"messages",
            "value":{"messages":[` + messages + `]}
        }]}]
    }`)
}
