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
}

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
	return s.repository.Verify(ctx, claims.VerificationID, s.now().UTC())
}
