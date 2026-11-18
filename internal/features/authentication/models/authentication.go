package models

import (
	"errors"
	"time"

	businessmodels "github.com/harzeezco/zeexco_commerce/internal/features/business/models"
)

var (
	ErrCredentialsNotFound = errors.New("authentication credentials not found")
	ErrRefreshTokenInvalid = errors.New("refresh token is invalid")
	ErrMembershipNotFound  = errors.New("business membership not found")
)

type Principal struct {
	UserID string
}

type CredentialUser struct {
	User
	PasswordHash string
}

type BusinessContext struct {
	Business   businessmodels.Business
	Membership businessmodels.Membership
}

type UserContext struct {
	User       User
	Businesses []BusinessContext
}

type RefreshSessionInput struct {
	UserID    string
	TokenHash []byte
	ExpiresAt time.Time
	CreatedAt time.Time
}

type RefreshSession struct {
	ID                  string
	UserID              string
	TokenHash           []byte
	CreatedAt           time.Time
	ExpiresAt           time.Time
	LastUsedAt          *time.Time
	RevokedAt           *time.Time
	ReplacedBySessionID *string
}

type RotationInput struct {
	CurrentTokenHash []byte
	NewTokenHash     []byte
	NewExpiresAt     time.Time
	RotatedAt        time.Time
}
