package emailverification

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"
)

const (
	defaultResendBaseURL = "https://api.resend.com"
	maxResendBodyBytes   = 64 << 10
)

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type ResendConfig struct {
	APIKey        string
	SecurityFrom  string
	LifecycleFrom string
	BaseURL       string
	HTTPClient    HTTPDoer
}

type ResendClient struct {
	apiKey        string
	securityFrom  string
	lifecycleFrom string
	endpoint      string
	httpClient    HTTPDoer
}

type ResendError struct {
	StatusCode int
}

func (e *ResendError) Error() string {
	return fmt.Sprintf("Resend email request failed with HTTP status %d", e.StatusCode)
}

func NewResendClient(cfg ResendConfig) (*ResendClient, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, errors.New("RESEND_API_KEY is required")
	}
	securityFrom, err := validSender("RESEND_SECURITY_FROM_EMAIL", cfg.SecurityFrom)
	if err != nil {
		return nil, err
	}
	lifecycleFrom, err := validSender("RESEND_LIFECYCLE_FROM_EMAIL", cfg.LifecycleFrom)
	if err != nil {
		return nil, err
	}
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = defaultResendBaseURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("RESEND_API_BASE_URL must be an absolute HTTPS URL")
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 10 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}
	return &ResendClient{
		apiKey: cfg.APIKey, securityFrom: securityFrom, lifecycleFrom: lifecycleFrom,
		endpoint: strings.TrimRight(baseURL, "/") + "/emails", httpClient: httpClient,
	}, nil
}

func validSender(name, value string) (string, error) {
	value = strings.TrimSpace(value)
	address, err := mail.ParseAddress(value)
	if err != nil || address.Address == "" || !strings.Contains(address.Address, "@") {
		return "", fmt.Errorf("%s must be a valid email sender", name)
	}
	return value, nil
}

func (c *ResendClient) Send(
	ctx context.Context,
	jobID string,
	purpose EmailPurpose,
	to string,
	subject string,
	htmlBody string,
	textBody string,
) (string, error) {
	if c == nil || strings.TrimSpace(jobID) == "" || strings.TrimSpace(to) == "" ||
		strings.TrimSpace(subject) == "" || strings.TrimSpace(htmlBody) == "" {
		return "", errors.New("complete Resend email input is required")
	}
	from, prefix, err := c.senderFor(purpose)
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(map[string]any{
		"from": from, "to": []string{to}, "subject": subject,
		"html": htmlBody, "text": textBody,
	})
	if err != nil {
		return "", fmt.Errorf("encode Resend email: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("create Resend email request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+c.apiKey)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", prefix+"/"+jobID)
	response, err := c.httpClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("send Resend email: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResendBodyBytes+1))
	if err != nil {
		return "", errors.New("read Resend response")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", &ResendError{StatusCode: response.StatusCode}
	}
	if len(body) > maxResendBodyBytes {
		return "", errors.New("Resend returned an oversized response")
	}
	var result struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(body, &result) != nil || strings.TrimSpace(result.ID) == "" {
		return "", errors.New("Resend returned a malformed response")
	}
	return result.ID, nil
}

func (c *ResendClient) senderFor(purpose EmailPurpose) (from, idempotencyPrefix string, err error) {
	switch purpose {
	case EmailPurposeSecurity:
		return c.securityFrom, "email-verification", nil
	case EmailPurposeLifecycle:
		return c.lifecycleFrom, "welcome", nil
	default:
		return "", "", errors.New("unsupported email purpose")
	}
}
