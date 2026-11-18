package models

import (
	"errors"

	businessmodels "github.com/harzeezco/zeexco_commerce/internal/features/business/models"
)

var ErrEmailAlreadyRegistered = errors.New("email is already registered")

var (
	ErrOnboardingInvalid  = errors.New("onboarding token is invalid")
	ErrOnboardingExpired  = errors.New("onboarding token has expired")
	ErrOnboardingConsumed = errors.New("onboarding token has already been used")
)

type RegistrationInput struct {
	OnboardingTokenHash []byte
	FirstName           string
	LastName            string
	Email               string
	PasswordHash        string
	BusinessName        string
}

type Registration struct {
	User       User
	Business   businessmodels.Business
	Membership businessmodels.Membership
}
