package cloudapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode"
)

const (
	defaultGraphAPIBaseURL = "https://graph.facebook.com"
	defaultHTTPTimeout     = 10 * time.Second
	maxResponseBodyBytes   = 64 << 10
	FlowModeDraft          = "draft"
	FlowModePublished      = "published"
)

var (
	ErrRecipientRequired   = errors.New("WhatsApp recipient is required")
	ErrMessageBodyRequired = errors.New("WhatsApp message body is required")
	ErrFlowConfiguration   = errors.New("WhatsApp Flow configuration is invalid")
	ErrMalformedResponse   = errors.New("Meta returned a malformed WhatsApp response")
)

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type ClientConfig struct {
	AccessToken   string
	PhoneNumberID string
	APIVersion    string
	FlowMode      string
	BaseURL       string
	HTTPClient    HTTPDoer
	Logger        *log.Logger
}

type Client struct {
	accessToken string
	endpoint    string
	flowMode    string
	httpClient  HTTPDoer
	logger      *log.Logger
}

type SendMessageResult struct {
	MessageID string
}

type ProviderError struct {
	StatusCode       int
	Code             int
	Subcode          int
	Type             string
	Message          string
	Details          string
	MessagingProduct string
	TraceID          string
}

func (e *ProviderError) Error() string {
	if e.Code == 0 && e.Subcode == 0 && e.Type == "" && e.Message == "" && e.Details == "" {
		return fmt.Sprintf("Meta WhatsApp request failed with HTTP status %d", e.StatusCode)
	}

	description := fmt.Sprintf(
		"Meta WhatsApp request failed with HTTP status %d, provider code %d, type %q",
		e.StatusCode,
		e.Code,
		e.Type,
	)
	if e.Subcode != 0 {
		description += fmt.Sprintf(", subcode %d", e.Subcode)
	}
	if e.Message != "" {
		description += fmt.Sprintf(", message %q", e.Message)
	}
	if e.Details != "" {
		description += fmt.Sprintf(", details %q", e.Details)
	}
	return description
}

type NetworkError struct {
	Err error
}

func (e *NetworkError) Error() string {
	return "Meta WhatsApp network request failed"
}

func (e *NetworkError) Unwrap() error {
	return e.Err
}

type sendTextRequest struct {
	MessagingProduct string          `json:"messaging_product"`
	RecipientType    string          `json:"recipient_type"`
	To               string          `json:"to"`
	Type             string          `json:"type"`
	Text             sendTextContent `json:"text"`
}

type sendTextContent struct {
	Body string `json:"body"`
}

type sendInteractiveContent struct {
	Text string `json:"text"`
}

type sendInteractiveRequest struct {
	MessagingProduct string              `json:"messaging_product"`
	RecipientType    string              `json:"recipient_type"`
	To               string              `json:"to"`
	Type             string              `json:"type"`
	Interactive      sendInteractiveBody `json:"interactive"`
}

type sendInteractiveBody struct {
	Type   string                 `json:"type"`
	Body   sendInteractiveContent `json:"body"`
	Action sendInteractiveAction  `json:"action"`
}

type sendInteractiveAction struct {
	Name       string                     `json:"name,omitempty"`
	Parameters *sendInteractiveParameters `json:"parameters,omitempty"`
	Buttons    []sendInteractiveButton    `json:"buttons,omitempty"`
}

type sendInteractiveParameters struct {
	DisplayText        string                 `json:"display_text,omitempty"`
	URL                string                 `json:"url,omitempty"`
	FlowMessageVersion string                 `json:"flow_message_version,omitempty"`
	FlowToken          string                 `json:"flow_token,omitempty"`
	FlowID             string                 `json:"flow_id,omitempty"`
	FlowCTA            string                 `json:"flow_cta,omitempty"`
	FlowAction         string                 `json:"flow_action,omitempty"`
	FlowActionPayload  *sendFlowActionPayload `json:"flow_action_payload,omitempty"`
	Mode               string                 `json:"mode,omitempty"`
}

type sendFlowActionPayload struct {
	Screen string         `json:"screen"`
	Data   map[string]any `json:"data,omitempty"`
}

type sendInteractiveButton struct {
	Type  string               `json:"type"`
	Reply sendInteractiveReply `json:"reply"`
}

type sendInteractiveReply struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type sendMessageResponse struct {
	Messages []struct {
		ID string `json:"id"`
	} `json:"messages"`
}

type providerErrorResponse struct {
	Error struct {
		Message      string `json:"message"`
		Type         string `json:"type"`
		Code         int    `json:"code"`
		ErrorSubcode int    `json:"error_subcode"`
		FBTraceID    string `json:"fbtrace_id"`
		ErrorData    struct {
			MessagingProduct string `json:"messaging_product"`
			Details          string `json:"details"`
		} `json:"error_data"`
	} `json:"error"`
}

func NewClient(cfg ClientConfig) (*Client, error) {
	if strings.TrimSpace(cfg.AccessToken) == "" {
		return nil, fmt.Errorf("WHATSAPP_ACCESS_TOKEN is required")
	}
	if err := validateNumericID("WHATSAPP_PHONE_NUMBER_ID", cfg.PhoneNumberID); err != nil {
		return nil, err
	}
	if err := validateGraphAPIVersion(cfg.APIVersion); err != nil {
		return nil, err
	}
	flowMode, err := parseFlowMode(cfg.FlowMode)
	if err != nil {
		return nil, err
	}

	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = defaultGraphAPIBaseURL
	}
	parsedBaseURL, err := url.Parse(baseURL)
	if err != nil || parsedBaseURL.Host == "" ||
		parsedBaseURL.User != nil || parsedBaseURL.RawQuery != "" || parsedBaseURL.Fragment != "" ||
		(parsedBaseURL.Scheme != "http" && parsedBaseURL.Scheme != "https") {
		return nil, fmt.Errorf("Meta Graph API base URL is invalid")
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: defaultHTTPTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}

	logger := cfg.Logger
	if logger == nil {
		logger = log.Default()
	}

	endpoint := baseURL + "/" + url.PathEscape(cfg.APIVersion) +
		"/" + url.PathEscape(cfg.PhoneNumberID) + "/messages"

	return &Client{
		accessToken: cfg.AccessToken,
		endpoint:    endpoint,
		flowMode:    flowMode,
		httpClient:  httpClient,
		logger:      logger,
	}, nil
}

func (c *Client) SendTextMessage(
	ctx context.Context,
	to string,
	body string,
) (SendMessageResult, error) {
	if strings.TrimSpace(to) == "" {
		c.logger.Printf("whatsapp outbound operation=send_text outcome=invalid_input reason=recipient_required")
		return SendMessageResult{}, ErrRecipientRequired
	}
	if strings.TrimSpace(body) == "" {
		c.logger.Printf("whatsapp outbound operation=send_text outcome=invalid_input reason=message_required")
		return SendMessageResult{}, ErrMessageBodyRequired
	}

	payload := sendTextRequest{
		MessagingProduct: "whatsapp",
		RecipientType:    "individual",
		To:               to,
		Type:             "text",
		Text: sendTextContent{
			Body: body,
		},
	}
	return c.sendPayload(ctx, payload, "send_text")
}

func (c *Client) SendText(ctx context.Context, to, body string) error {
	_, err := c.SendTextMessage(ctx, to, body)
	return err
}

func (c *Client) SendCTAURLMessage(ctx context.Context, to, body, displayText, targetURL string) error {
	if strings.TrimSpace(to) == "" {
		return ErrRecipientRequired
	}
	if strings.TrimSpace(body) == "" || strings.TrimSpace(displayText) == "" || len(displayText) > 20 {
		return ErrMessageBodyRequired
	}
	parsed, err := url.Parse(targetURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return errors.New("WhatsApp CTA URL must be absolute HTTPS")
	}
	payload := sendInteractiveRequest{
		MessagingProduct: "whatsapp", RecipientType: "individual", To: to, Type: "interactive",
		Interactive: sendInteractiveBody{
			Type: "cta_url", Body: sendInteractiveContent{Text: body},
			Action: sendInteractiveAction{
				Name:       "cta_url",
				Parameters: &sendInteractiveParameters{DisplayText: displayText, URL: targetURL},
			},
		},
	}
	_, err = c.sendPayload(ctx, payload, "send_cta_url")
	return err
}

func (c *Client) SendFlowMessage(ctx context.Context, to, body, cta, flowID, flowToken string) error {
	if strings.TrimSpace(to) == "" {
		return ErrRecipientRequired
	}
	if strings.TrimSpace(body) == "" || strings.TrimSpace(cta) == "" || len(cta) > 30 {
		return ErrMessageBodyRequired
	}
	if validateNumericID("WHATSAPP_MERCHANT_REGISTRATION_FLOW_ID", flowID) != nil || strings.TrimSpace(flowToken) == "" {
		return ErrFlowConfiguration
	}
	payload := sendInteractiveRequest{
		MessagingProduct: "whatsapp", RecipientType: "individual", To: to, Type: "interactive",
		Interactive: sendInteractiveBody{
			Type: "flow", Body: sendInteractiveContent{Text: body},
			Action: sendInteractiveAction{
				Name: "flow",
				Parameters: &sendInteractiveParameters{
					FlowMessageVersion: "3", FlowToken: flowToken, FlowID: flowID,
					FlowCTA: cta, FlowAction: "navigate", Mode: c.flowMode,
					FlowActionPayload: &sendFlowActionPayload{
						Screen: "REGISTRATION",
						Data:   map[string]any{"has_error": false, "error_message": ""},
					},
				},
			},
		},
	}
	_, err := c.sendPayload(ctx, payload, "send_registration_flow")
	return err
}

func parseFlowMode(value string) (string, error) {
	mode := strings.ToLower(strings.TrimSpace(value))
	if mode == "" {
		return FlowModePublished, nil
	}
	switch mode {
	case FlowModeDraft, FlowModePublished:
		return mode, nil
	default:
		return "", errors.New("WHATSAPP_FLOW_MODE must be draft or published")
	}
}

func (c *Client) SendReplyButtonMessage(ctx context.Context, to, body, buttonID, title string) error {
	if strings.TrimSpace(to) == "" {
		return ErrRecipientRequired
	}
	if strings.TrimSpace(body) == "" || strings.TrimSpace(buttonID) == "" || strings.TrimSpace(title) == "" || len(title) > 20 {
		return ErrMessageBodyRequired
	}
	payload := sendInteractiveRequest{
		MessagingProduct: "whatsapp", RecipientType: "individual", To: to, Type: "interactive",
		Interactive: sendInteractiveBody{
			Type: "button", Body: sendInteractiveContent{Text: body},
			Action: sendInteractiveAction{Buttons: []sendInteractiveButton{{
				Type: "reply", Reply: sendInteractiveReply{ID: buttonID, Title: title},
			}}},
		},
	}
	_, err := c.sendPayload(ctx, payload, "send_reply_button")
	return err
}

func (c *Client) sendPayload(ctx context.Context, payload any, operation string) (SendMessageResult, error) {

	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		c.logger.Printf("whatsapp outbound operation=%s outcome=request_encode_failed", operation)
		return SendMessageResult{}, fmt.Errorf("encode Meta WhatsApp request: %w", err)
	}

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.endpoint,
		bytes.NewReader(encodedPayload),
	)
	if err != nil {
		c.logger.Printf("whatsapp outbound operation=%s outcome=request_create_failed", operation)
		return SendMessageResult{}, fmt.Errorf("create Meta WhatsApp request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+c.accessToken)
	request.Header.Set("Content-Type", "application/json")

	response, err := c.httpClient.Do(request)
	if err != nil {
		c.logger.Printf("whatsapp outbound operation=%s outcome=network_error", operation)
		return SendMessageResult{}, &NetworkError{Err: err}
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBodyBytes+1))
	if err != nil {
		c.logger.Printf(
			"whatsapp outbound operation=%s outcome=response_read_failed http_status=%d", operation,
			response.StatusCode,
		)
		return SendMessageResult{}, fmt.Errorf("read Meta WhatsApp response")
	}
	if len(responseBody) > maxResponseBodyBytes {
		c.logger.Printf(
			"whatsapp outbound operation=%s outcome=malformed_response http_status=%d", operation,
			response.StatusCode,
		)
		return SendMessageResult{}, ErrMalformedResponse
	}

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		providerError := decodeProviderError(
			response.StatusCode,
			responseBody,
			append(payloadSensitiveValues(encodedPayload), c.accessToken)...,
		)
		c.logger.Printf(
			"whatsapp outbound operation=%s outcome=provider_error http_status=%d provider_code=%d provider_subcode=%d provider_type=%q provider_message=%q provider_details=%q provider_trace_id=%q", operation,
			providerError.StatusCode,
			providerError.Code,
			providerError.Subcode,
			providerError.Type,
			providerError.Message,
			providerError.Details,
			providerError.TraceID,
		)
		return SendMessageResult{}, providerError
	}

	var decodedResponse sendMessageResponse
	if err := json.Unmarshal(responseBody, &decodedResponse); err != nil ||
		len(decodedResponse.Messages) == 0 ||
		strings.TrimSpace(decodedResponse.Messages[0].ID) == "" {
		c.logger.Printf(
			"whatsapp outbound operation=%s outcome=malformed_response http_status=%d", operation,
			response.StatusCode,
		)
		return SendMessageResult{}, ErrMalformedResponse
	}

	c.logger.Printf(
		"whatsapp outbound operation=%s outcome=success http_status=%d", operation,
		response.StatusCode,
	)
	return SendMessageResult{MessageID: decodedResponse.Messages[0].ID}, nil
}

func decodeProviderError(statusCode int, body []byte, sensitiveValues ...string) *ProviderError {
	providerError := &ProviderError{StatusCode: statusCode}

	var decoded providerErrorResponse
	if err := json.Unmarshal(body, &decoded); err == nil {
		providerError.Code = decoded.Error.Code
		providerError.Subcode = decoded.Error.ErrorSubcode
		providerError.Type = sanitizeProviderErrorType(decoded.Error.Type)
		providerError.Message = sanitizeProviderDiagnostic(decoded.Error.Message, sensitiveValues)
		providerError.Details = sanitizeProviderDiagnostic(decoded.Error.ErrorData.Details, sensitiveValues)
		providerError.MessagingProduct = sanitizeProviderErrorType(decoded.Error.ErrorData.MessagingProduct)
		providerError.TraceID = sanitizeProviderIdentifier(decoded.Error.FBTraceID, 128)
	}

	return providerError
}

func sanitizeProviderDiagnostic(value string, sensitiveValues []string) string {
	value = strings.Map(func(character rune) rune {
		if character == '\n' || character == '\r' || character == '\t' {
			return ' '
		}
		if !unicode.IsPrint(character) {
			return -1
		}
		return character
	}, value)
	value = strings.Join(strings.Fields(value), " ")

	values := append([]string(nil), sensitiveValues...)
	sort.Slice(values, func(left, right int) bool { return len(values[left]) > len(values[right]) })
	for _, sensitive := range values {
		sensitive = strings.TrimSpace(sensitive)
		if len(sensitive) >= 4 {
			value = strings.ReplaceAll(value, sensitive, "[redacted]")
		}
	}
	if len(value) > 512 {
		value = value[:512]
	}
	return value
}

func payloadSensitiveValues(encodedPayload []byte) []string {
	var value any
	if json.Unmarshal(encodedPayload, &value) != nil {
		return nil
	}
	values := make([]string, 0)
	collectPayloadStrings(value, &values)
	return values
}

func collectPayloadStrings(value any, values *[]string) {
	switch typed := value.(type) {
	case string:
		switch typed {
		case "whatsapp", "individual", "text", "interactive", "cta_url", "button", "reply":
			return
		}
		*values = append(*values, typed)
	case []any:
		for _, item := range typed {
			collectPayloadStrings(item, values)
		}
	case map[string]any:
		for _, item := range typed {
			collectPayloadStrings(item, values)
		}
	}
}

func sanitizeProviderIdentifier(value string, maximumLength int) string {
	if value == "" || len(value) > maximumLength {
		return ""
	}
	for _, character := range value {
		if !unicode.IsLetter(character) && !unicode.IsDigit(character) &&
			character != '_' && character != '-' && character != '.' {
			return ""
		}
	}
	return value
}

func sanitizeProviderErrorType(value string) string {
	return sanitizeProviderIdentifier(value, 64)
}

func validatePathSegment(name, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", name)
	}
	if strings.ContainsAny(value, "/\\?#") {
		return fmt.Errorf("%s is invalid", name)
	}

	return nil
}

func validateNumericID(name, value string) error {
	if err := validatePathSegment(name, value); err != nil {
		return err
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return fmt.Errorf("%s is invalid", name)
		}
	}
	return nil
}

func validateGraphAPIVersion(value string) error {
	if err := validatePathSegment("META_GRAPH_API_VERSION", value); err != nil {
		return err
	}
	parts := strings.Split(strings.TrimPrefix(value, "v"), ".")
	if !strings.HasPrefix(value, "v") || len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return errors.New("META_GRAPH_API_VERSION must use the v<major>.<minor> format")
	}
	for _, part := range parts {
		for _, character := range part {
			if character < '0' || character > '9' {
				return errors.New("META_GRAPH_API_VERSION must use the v<major>.<minor> format")
			}
		}
	}
	return nil
}
