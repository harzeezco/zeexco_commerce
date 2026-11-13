package emailverification

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
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
	APIKey     string
	From       string
	BaseURL    string
	HTTPClient HTTPDoer
}

type ResendClient struct {
	apiKey     string
	from       string
	endpoint   string
	httpClient HTTPDoer
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
	if strings.TrimSpace(cfg.From) == "" || !strings.Contains(cfg.From, "@") {
		return nil, errors.New("RESEND_FROM_EMAIL is required")
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
		apiKey: cfg.APIKey, from: cfg.From,
		endpoint: strings.TrimRight(baseURL, "/") + "/emails", httpClient: httpClient,
	}, nil
}

func (c *ResendClient) Send(
	ctx context.Context,
	jobID string,
	to string,
	subject string,
	htmlBody string,
	textBody string,
) (string, error) {
	if c == nil || strings.TrimSpace(jobID) == "" || strings.TrimSpace(to) == "" ||
		strings.TrimSpace(subject) == "" || strings.TrimSpace(htmlBody) == "" {
		return "", errors.New("complete Resend email input is required")
	}
	payload, err := json.Marshal(map[string]any{
		"from": c.from, "to": []string{to}, "subject": subject,
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
	request.Header.Set("Idempotency-Key", "email-verification/"+jobID)
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
