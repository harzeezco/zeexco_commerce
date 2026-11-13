package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/harzeezco/zeexco_commerce/internal/features/authentication/models"
	"github.com/harzeezco/zeexco_commerce/internal/features/authentication/security"
)

var (
	ErrInvalidCredentials = errors.New("email or password is invalid")
	ErrInvalidRefresh     = errors.New("refresh session is invalid")
)

type AuthenticationRepository interface {
	FindCredentialUser(context.Context, string) (models.CredentialUser, error)
	CreateRefreshSession(context.Context, models.RefreshSessionInput) (models.RefreshSession, error)
	RotateRefreshSession(context.Context, models.RotationInput) (models.RefreshSession, error)
	RevokeRefreshSession(context.Context, []byte, time.Time) (string, error)
	GetUserContext(context.Context, string) (models.UserContext, error)
}

type PasswordService interface {
	Hash(password string) (string, error)
	Verify(hash, password string) error
}

type AccessTokenService interface {
	Mint(userID string) (string, error)
	Verify(raw string) (models.Principal, error)
	Lifetime() time.Duration
}

type LoginCommand struct {
	Email    string
	Password string
}

type AuthenticationResult struct {
	AccessToken  string
	ExpiresIn    int64
	RefreshToken string
	Context      models.UserContext
}

type AuthenticationService struct {
	repository AuthenticationRepository
	passwords  PasswordService
	tokens     AccessTokenService
	refreshTTL time.Duration
	dummyHash  string
	now        func() time.Time
}

func NewAuthenticationService(
	repository AuthenticationRepository,
	passwords PasswordService,
	tokens AccessTokenService,
	refreshTTL time.Duration,
) (*AuthenticationService, error) {
	if repository == nil || passwords == nil || tokens == nil || refreshTTL <= 0 {
		return nil, errors.New("authentication repository, password service, token service, and positive refresh lifetime are required")
	}
	dummyHash, err := passwords.Hash("zeexco-dummy-credential-check")
	if err != nil {
		return nil, fmt.Errorf("initialize credential timing defense: %w", err)
	}
	return &AuthenticationService{
		repository: repository, passwords: passwords, tokens: tokens,
		refreshTTL: refreshTTL, dummyHash: dummyHash, now: time.Now,
	}, nil
}

func (s *AuthenticationService) Login(
	ctx context.Context,
	command LoginCommand,
) (AuthenticationResult, error) {
	email := normalizeEmail(command.Email)
	user, err := s.repository.FindCredentialUser(ctx, email)
	if errors.Is(err, models.ErrCredentialsNotFound) {
		_ = s.passwords.Verify(s.dummyHash, command.Password)
		return AuthenticationResult{}, ErrInvalidCredentials
	}
	if err != nil {
		return AuthenticationResult{}, fmt.Errorf("find login user: %w", err)
	}
	if s.passwords.Verify(user.PasswordHash, command.Password) != nil || user.Status != models.UserStatusActive {
		return AuthenticationResult{}, ErrInvalidCredentials
	}

	return s.createAuthentication(ctx, user.ID)
}

func (s *AuthenticationService) Refresh(
	ctx context.Context,
	rawRefreshToken string,
) (AuthenticationResult, error) {
	if rawRefreshToken == "" {
		return AuthenticationResult{}, ErrInvalidRefresh
	}
	newRaw, newHash, err := security.NewRefreshToken()
	if err != nil {
		return AuthenticationResult{}, fmt.Errorf("generate replacement refresh token: %w", err)
	}
	now := s.now().UTC()
	replacement, err := s.repository.RotateRefreshSession(ctx, models.RotationInput{
		CurrentTokenHash: security.HashRefreshToken(rawRefreshToken),
		NewTokenHash:     newHash, NewExpiresAt: now.Add(s.refreshTTL), RotatedAt: now,
	})
	if errors.Is(err, models.ErrRefreshTokenInvalid) {
		return AuthenticationResult{}, ErrInvalidRefresh
	}
	if err != nil {
		return AuthenticationResult{}, fmt.Errorf("rotate refresh session: %w", err)
	}
	return s.authenticationForExistingSession(ctx, replacement.UserID, newRaw)
}

func (s *AuthenticationService) Logout(ctx context.Context, rawRefreshToken string) error {
	if rawRefreshToken == "" {
		return nil
	}
	_, err := s.repository.RevokeRefreshSession(
		ctx,
		security.HashRefreshToken(rawRefreshToken),
		s.now().UTC(),
	)
	if errors.Is(err, models.ErrRefreshTokenInvalid) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("revoke refresh session: %w", err)
	}
	return nil
}

func (s *AuthenticationService) Me(ctx context.Context, principal models.Principal) (models.UserContext, error) {
	if !models.IsUUID(principal.UserID) {
		return models.UserContext{}, errors.New("authenticated principal has invalid user ID")
	}
	return s.repository.GetUserContext(ctx, principal.UserID)
}

func (s *AuthenticationService) createAuthentication(
	ctx context.Context,
	userID string,
) (AuthenticationResult, error) {
	contextResult, err := s.repository.GetUserContext(ctx, userID)
	if err != nil {
		return AuthenticationResult{}, fmt.Errorf("load login context: %w", err)
	}
	accessToken, err := s.tokens.Mint(userID)
	if err != nil {
		return AuthenticationResult{}, fmt.Errorf("mint access token: %w", err)
	}
	rawRefresh, refreshHash, err := security.NewRefreshToken()
	if err != nil {
		return AuthenticationResult{}, fmt.Errorf("generate refresh token: %w", err)
	}
	now := s.now().UTC()
	if _, err := s.repository.CreateRefreshSession(ctx, models.RefreshSessionInput{
		UserID: userID, TokenHash: refreshHash,
		CreatedAt: now, ExpiresAt: now.Add(s.refreshTTL),
	}); err != nil {
		return AuthenticationResult{}, fmt.Errorf("persist refresh session: %w", err)
	}
	return AuthenticationResult{
		AccessToken: accessToken, ExpiresIn: int64(s.tokens.Lifetime().Seconds()),
		RefreshToken: rawRefresh, Context: contextResult,
	}, nil
}

func (s *AuthenticationService) authenticationForExistingSession(
	ctx context.Context,
	userID string,
	refreshToken string,
) (AuthenticationResult, error) {
	contextResult, err := s.repository.GetUserContext(ctx, userID)
	if err != nil {
		return AuthenticationResult{}, fmt.Errorf("load refreshed user context: %w", err)
	}
	accessToken, err := s.tokens.Mint(userID)
	if err != nil {
		return AuthenticationResult{}, fmt.Errorf("mint refreshed access token: %w", err)
	}
	return AuthenticationResult{
		AccessToken: accessToken, ExpiresIn: int64(s.tokens.Lifetime().Seconds()),
		RefreshToken: refreshToken, Context: contextResult,
	}, nil
}
