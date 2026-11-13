package services

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	authmodels "github.com/harzeezco/zeexco_commerce/internal/features/authentication/models"
	businessmodels "github.com/harzeezco/zeexco_commerce/internal/features/business/models"
)

func TestRegisterNormalizesHashesAndPersistsRegistration(t *testing.T) {
	repository := &fakeRegistrationRepository{registration: testRegistration()}
	hasher := &fakePasswordHasher{hash: "secure-password-hash"}
	service := newTestRegisterService(t, repository, hasher)

	registration, err := service.Register(context.Background(), RegisterCommand{
		FirstName:    "  Ada   Maria ",
		LastName:     " Lovelace ",
		BusinessName: "  Analytical   Commerce ",
		Email:        " ADA@Example.COM ",
		Password:     "correct horse battery staple",
	})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if registration != repository.registration {
		t.Fatal("Register() did not return repository registration")
	}
	if repository.input.FirstName != "Ada Maria" ||
		repository.input.LastName != "Lovelace" ||
		repository.input.BusinessName != "Analytical Commerce" ||
		repository.input.Email != "ada@example.com" {
		t.Fatalf("normalized input = %#v", repository.input)
	}
	if repository.input.PasswordHash != hasher.hash || repository.input.PasswordHash == hasher.password {
		t.Fatalf("password hash input = %q", repository.input.PasswordHash)
	}
	if hasher.password != "correct horse battery staple" {
		t.Fatal("password hasher did not receive exact password")
	}
}

func TestRegisterRejectsInvalidInputBeforeHashingOrPersistence(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*RegisterCommand)
		field  string
	}{
		{name: "empty first name", mutate: func(c *RegisterCommand) { c.FirstName = " " }, field: "first_name"},
		{name: "long first name", mutate: func(c *RegisterCommand) { c.FirstName = strings.Repeat("a", 101) }, field: "first_name"},
		{name: "empty last name", mutate: func(c *RegisterCommand) { c.LastName = "" }, field: "last_name"},
		{name: "empty business", mutate: func(c *RegisterCommand) { c.BusinessName = "" }, field: "business_name"},
		{name: "long business", mutate: func(c *RegisterCommand) { c.BusinessName = strings.Repeat("b", 201) }, field: "business_name"},
		{name: "invalid email", mutate: func(c *RegisterCommand) { c.Email = "not-an-email" }, field: "email"},
		{name: "short password", mutate: func(c *RegisterCommand) { c.Password = "too-short" }, field: "password"},
		{name: "long password", mutate: func(c *RegisterCommand) { c.Password = strings.Repeat("p", 73) }, field: "password"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &fakeRegistrationRepository{}
			hasher := &fakePasswordHasher{hash: "unused"}
			service := newTestRegisterService(t, repository, hasher)
			command := validRegisterCommand()
			test.mutate(&command)

			_, err := service.Register(context.Background(), command)
			var validationError *ValidationError
			if !errors.As(err, &validationError) || validationError.Field != test.field {
				t.Fatalf("error = %v, want validation field %q", err, test.field)
			}
			if hasher.calls != 0 || repository.calls != 0 {
				t.Fatalf("hasher calls = %d, repository calls = %d", hasher.calls, repository.calls)
			}
		})
	}
}

func TestRegisterPreservesDuplicateEmailError(t *testing.T) {
	repository := &fakeRegistrationRepository{err: authmodels.ErrEmailAlreadyRegistered}
	service := newTestRegisterService(t, repository, &fakePasswordHasher{hash: "hash"})

	_, err := service.Register(context.Background(), validRegisterCommand())
	if !errors.Is(err, authmodels.ErrEmailAlreadyRegistered) {
		t.Fatalf("Register() error = %v, want duplicate email", err)
	}
}

func TestRegisterReturnsHasherAndRepositoryFailures(t *testing.T) {
	hashError := errors.New("hash failure")
	service := newTestRegisterService(t, &fakeRegistrationRepository{}, &fakePasswordHasher{err: hashError})
	if _, err := service.Register(context.Background(), validRegisterCommand()); !errors.Is(err, hashError) {
		t.Fatalf("hash error = %v", err)
	}

	repositoryError := errors.New("repository failure")
	service = newTestRegisterService(t, &fakeRegistrationRepository{err: repositoryError}, &fakePasswordHasher{hash: "hash"})
	if _, err := service.Register(context.Background(), validRegisterCommand()); !errors.Is(err, repositoryError) {
		t.Fatalf("repository error = %v", err)
	}
}

func newTestRegisterService(t *testing.T, repository RegistrationRepository, hasher PasswordHasher) *RegisterService {
	t.Helper()
	service, err := NewRegisterService(repository, hasher)
	if err != nil {
		t.Fatalf("NewRegisterService() error = %v", err)
	}
	return service
}

func validRegisterCommand() RegisterCommand {
	return RegisterCommand{
		FirstName: "Ada", LastName: "Lovelace", BusinessName: "Analytical Commerce",
		Email: "ada@example.com", Password: "correct horse battery staple",
	}
}

func testRegistration() *authmodels.Registration {
	now := time.Unix(1800000000, 0).UTC()
	return &authmodels.Registration{
		User: authmodels.User{
			ID: "00000000-0000-4000-8000-000000000001", FirstName: "Ada", LastName: "Lovelace",
			Email: "ada@example.com", Status: authmodels.UserStatusActive, CreatedAt: now, UpdatedAt: now,
		},
		Business: businessmodels.Business{
			ID: "00000000-0000-4000-8000-000000000002", Name: "Analytical Commerce",
			Status: businessmodels.StatusActive, CreatedAt: now, UpdatedAt: now,
		},
		Membership: businessmodels.Membership{
			ID:         "00000000-0000-4000-8000-000000000003",
			BusinessID: "00000000-0000-4000-8000-000000000002",
			UserID:     "00000000-0000-4000-8000-000000000001", Role: businessmodels.RoleOwner, CreatedAt: now,
		},
	}
}

type fakeRegistrationRepository struct {
	registration *authmodels.Registration
	input        authmodels.RegistrationInput
	err          error
	calls        int
}

func (r *fakeRegistrationRepository) Register(_ context.Context, input authmodels.RegistrationInput) (*authmodels.Registration, error) {
	r.calls++
	r.input = input
	return r.registration, r.err
}

type fakePasswordHasher struct {
	hash     string
	password string
	err      error
	calls    int
}

func (h *fakePasswordHasher) Hash(password string) (string, error) {
	h.calls++
	h.password = password
	return h.hash, h.err
}
