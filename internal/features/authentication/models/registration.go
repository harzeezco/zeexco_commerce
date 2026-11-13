package models

import (
	"errors"

	businessmodels "github.com/harzeezco/zeexco_commerce/internal/features/business/models"
)

var ErrEmailAlreadyRegistered = errors.New("email is already registered")

type RegistrationInput struct {
	FirstName    string
	LastName     string
	Email        string
	PasswordHash string
	BusinessName string
}

type Registration struct {
	User       User
	Business   businessmodels.Business
	Membership businessmodels.Membership
}
