package emailverification

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"
)

type VerificationRepository interface {
	Verify(context.Context, string, time.Time) error
	RequestVerification(context.Context, string, time.Time, time.Time) (bool, error)
}

type Service struct {
	repository        VerificationRepository
	tokens            *TokenManager
	now               func() time.Time
	ttl               time.Duration
	notifier          interface{ Notify() }
	whatsAppReturnURL string
}

type VerificationResult struct {
	ContinueInWhatsApp bool
	RedirectURL        string
}

// The destination comes from trusted server configuration, never the browser.
func (s *Service) SetWhatsAppReturnURL(raw string) error {
	if raw == "" || raw == "https://wa.me/" {
		return nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host != "wa.me" || parsed.User != nil || parsed.Fragment != "" {
		return errors.New("WHATSAPP_ONBOARDING_RETURN_URL must be an HTTPS wa.me link to the public WhatsApp number")
	}
	number := strings.TrimPrefix(parsed.Path, "/")
	if len(number) < 7 || len(number) > 15 || number[0] == '0' {
		return errors.New("WhatsApp return link requires an international phone number")
	}
	for _, digit := range number {
		if digit < '0' || digit > '9' {
			return errors.New("WhatsApp return link requires digits only")
		}
	}
	parsed.RawQuery = url.Values{"text": {"Continue"}}.Encode()
	s.whatsAppReturnURL = parsed.String()
	return nil
}

func (s *Service) SetNotifier(notifier interface{ Notify() }) { s.notifier = notifier }

func NewService(repository VerificationRepository, tokens *TokenManager, ttl time.Duration) (*Service, error) {
	if repository == nil || tokens == nil || ttl <= 0 {
		return nil, errors.New("email verification repository, token manager, and positive lifetime are required")
	}
	return &Service{repository: repository, tokens: tokens, ttl: ttl, now: time.Now}, nil
}

func (s *Service) Request(ctx context.Context, userID string) error {
	now := s.now().UTC()
	queued, err := s.repository.RequestVerification(ctx, userID, now, now.Add(s.ttl))
	if err != nil {
		return err
	}
	if queued && s.notifier != nil {
		s.notifier.Notify()
	}
	return nil
}

func (s *Service) Verify(ctx context.Context, rawToken string) error {
	_, err := s.VerifyWithResult(ctx, rawToken)
	return err
}

func (s *Service) VerifyWithResult(ctx context.Context, rawToken string) (VerificationResult, error) {
	claims, err := s.tokens.Verify(rawToken)
	if err != nil {
		return VerificationResult{}, ErrInvalidToken
	}
	now := s.now().UTC()
	if !now.Before(claims.ExpiresAt) {
		return VerificationResult{}, ErrVerificationExpired
	}
	var continuation bool
	if repository, ok := s.repository.(interface {
		VerifyWithContinuation(context.Context, string, time.Time) (bool, error)
	}); ok {
		continuation, err = repository.VerifyWithContinuation(ctx, claims.VerificationID, now)
	} else {
		err = s.repository.Verify(ctx, claims.VerificationID, now)
	}
	if err != nil {
		return VerificationResult{}, err
	}
	if s.notifier != nil {
		s.notifier.Notify()
	}
	return VerificationResult{ContinueInWhatsApp: continuation, RedirectURL: s.whatsAppReturnURL}, nil
}
