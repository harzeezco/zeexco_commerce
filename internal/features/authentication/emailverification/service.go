package emailverification

import (
	"context"
	"errors"
	"time"
)

type VerificationRepository interface {
	Verify(context.Context, string, time.Time) error
}

type Service struct {
	repository VerificationRepository
	tokens     *TokenManager
	now        func() time.Time
	notifier   interface{ Notify() }
}

func (s *Service) SetNotifier(notifier interface{ Notify() }) { s.notifier = notifier }

func NewService(repository VerificationRepository, tokens *TokenManager) (*Service, error) {
	if repository == nil || tokens == nil {
		return nil, errors.New("email verification repository and token manager are required")
	}
	return &Service{repository: repository, tokens: tokens, now: time.Now}, nil
}

func (s *Service) Verify(ctx context.Context, rawToken string) error {
	claims, err := s.tokens.Verify(rawToken)
	if err != nil {
		return ErrInvalidToken
	}
	if err := s.repository.Verify(ctx, claims.VerificationID, s.now().UTC()); err != nil {
		return err
	}
	if s.notifier != nil {
		s.notifier.Notify()
	}
	return nil
}
