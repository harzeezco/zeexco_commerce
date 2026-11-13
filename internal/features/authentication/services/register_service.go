package services

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"unicode/utf8"

	"github.com/harzeezco/zeexco_commerce/internal/features/authentication/models"
)

const (
	NextStepAddFirstProduct = "ADD_FIRST_PRODUCT"
	NextStepVerifyEmail     = "VERIFY_EMAIL"
	minimumPasswordBytes    = 12
	maximumPasswordBytes    = 72
)

var (
	ErrRegistrationRepositoryRequired = errors.New("registration repository is required")
	ErrPasswordHasherRequired         = errors.New("password hasher is required")
)

type RegisterCommand struct {
	FirstName    string
	LastName     string
	BusinessName string
	Email        string
	Password     string
}

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}

type RegistrationRepository interface {
	Register(context.Context, models.RegistrationInput) (*models.Registration, error)
}

type PasswordHasher interface {
	Hash(password string) (string, error)
}

type RegistrationJobNotifier interface {
	Notify()
}

type RegisterService struct {
	repository RegistrationRepository
	hasher     PasswordHasher
	notifier   RegistrationJobNotifier
}

func (s *RegisterService) SetNotifier(notifier RegistrationJobNotifier) { s.notifier = notifier }

func NewRegisterService(
	repository RegistrationRepository,
	hasher PasswordHasher,
) (*RegisterService, error) {
	if repository == nil {
		return nil, ErrRegistrationRepositoryRequired
	}
	if hasher == nil {
		return nil, ErrPasswordHasherRequired
	}
	return &RegisterService{repository: repository, hasher: hasher}, nil
}

func (s *RegisterService) Register(
	ctx context.Context,
	command RegisterCommand,
) (*models.Registration, error) {
	input, err := s.Prepare(command)
	if err != nil {
		return nil, err
	}

	registration, err := s.repository.Register(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("persist registration: %w", err)
	}
	if s.notifier != nil {
		s.notifier.Notify()
	}
	return registration, nil
}

// Prepare applies the canonical browser-registration validation and password
// hashing rules without persisting anything. Alternate trusted entry paths can
// reuse these rules while owning a larger atomic transaction.
func (s *RegisterService) Prepare(command RegisterCommand) (models.RegistrationInput, error) {
	normalized, err := normalizeRegistration(command)
	if err != nil {
		return models.RegistrationInput{}, err
	}

	passwordHash, err := s.hasher.Hash(command.Password)
	if err != nil {
		return models.RegistrationInput{}, fmt.Errorf("hash registration password: %w", err)
	}

	return models.RegistrationInput{
		FirstName:    normalized.FirstName,
		LastName:     normalized.LastName,
		BusinessName: normalized.BusinessName,
		Email:        normalized.Email,
		PasswordHash: passwordHash,
	}, nil
}

func normalizeRegistration(command RegisterCommand) (RegisterCommand, error) {
	command.FirstName = normalizeName(command.FirstName)
	command.LastName = normalizeName(command.LastName)
	command.BusinessName = normalizeName(command.BusinessName)
	command.Email = normalizeEmail(command.Email)

	if command.FirstName == "" {
		return RegisterCommand{}, validationError("first_name", "first name is required")
	}
	if utf8.RuneCountInString(command.FirstName) > 100 {
		return RegisterCommand{}, validationError("first_name", "first name must be 100 characters or fewer")
	}
	if command.LastName == "" {
		return RegisterCommand{}, validationError("last_name", "last name is required")
	}
	if utf8.RuneCountInString(command.LastName) > 100 {
		return RegisterCommand{}, validationError("last_name", "last name must be 100 characters or fewer")
	}
	if command.BusinessName == "" {
		return RegisterCommand{}, validationError("business_name", "business name is required")
	}
	if utf8.RuneCountInString(command.BusinessName) > 200 {
		return RegisterCommand{}, validationError("business_name", "business name must be 200 characters or fewer")
	}
	if len(command.Email) > 255 || !validEmail(command.Email) {
		return RegisterCommand{}, validationError("email", "email address is invalid")
	}
	if len(command.Password) < minimumPasswordBytes {
		return RegisterCommand{}, validationError("password", "password must be at least 12 bytes")
	}
	if len(command.Password) > maximumPasswordBytes {
		return RegisterCommand{}, validationError("password", "password must be 72 bytes or fewer")
	}

	return command, nil
}

func normalizeEmail(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeName(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func validEmail(value string) bool {
	if value == "" {
		return false
	}
	parsed, err := mail.ParseAddress(value)
	return err == nil && parsed.Address == value
}

func validationError(field, message string) error {
	return &ValidationError{Field: field, Message: message}
}
