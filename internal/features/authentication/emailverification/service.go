package emailverification

import (
	"context"
	"errors"
	"time"
)

type VerificationRepository interface {
	Verify(context.Context, string, time.Time) error
	RequestVerification(context.Context, string, time.Time, time.Time) (bool, error)
}

type Service struct {
	repository VerificationRepository
	tokens     *TokenManager
	now        func() time.Time
	ttl        time.Duration
	notifier   interface{ Notify() }
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
	claims, err := s.tokens.Verify(rawToken)
	if err != nil {
		return ErrInvalidToken
	}
	now := s.now().UTC()
	if !now.Before(claims.ExpiresAt) {
		return ErrVerificationExpired
	}
	if err := s.repository.Verify(ctx, claims.VerificationID, now); err != nil {
		return err
	}
	if s.notifier != nil {
		s.notifier.Notify()
	}
	return nil
}
