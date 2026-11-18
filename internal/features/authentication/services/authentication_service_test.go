package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/harzeezco/zeexco_commerce/internal/features/authentication/models"
	businessmodels "github.com/harzeezco/zeexco_commerce/internal/features/business/models"
)

func TestAuthenticationServiceLoginCreatesAuthentication(t *testing.T) {
	repository := newFakeAuthenticationRepository()
	repository.credential = credentialUser(models.UserStatusActive)
	service := newTestAuthenticationService(t, repository)

	result, err := service.Login(context.Background(), LoginCommand{
		Email: " MERCHANT@Example.COM ", Password: "correct-password",
	})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if repository.email != "merchant@example.com" || repository.created.UserID != testAuthUserID {
		t.Fatalf("email=%q created=%#v", repository.email, repository.created)
	}
	if result.AccessToken != "access-token" || result.RefreshToken == "" || result.ExpiresIn != 900 {
		t.Fatalf("result = %#v", result)
	}
	if string(repository.created.TokenHash) == result.RefreshToken || len(repository.created.TokenHash) != 32 {
		t.Fatal("repository received raw or invalid refresh token hash")
	}
}

func TestAuthenticationServiceLoginRejectsIncorrectUnknownAndSuspendedUsers(t *testing.T) {
	tests := []struct {
		name       string
		credential models.CredentialUser
		findErr    error
		password   string
	}{
		{name: "incorrect password", credential: credentialUser(models.UserStatusActive), password: "wrong"},
		{name: "unknown email", findErr: models.ErrCredentialsNotFound, password: "wrong"},
		{name: "suspended", credential: credentialUser(models.UserStatusSuspended), password: "correct-password"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := newFakeAuthenticationRepository()
			repository.credential, repository.findErr = test.credential, test.findErr
			service := newTestAuthenticationService(t, repository)
			_, err := service.Login(context.Background(), LoginCommand{
				Email: "merchant@example.com", Password: test.password,
			})
			if !errors.Is(err, ErrInvalidCredentials) {
				t.Fatalf("Login() error = %v", err)
			}
			if repository.createCalls != 0 {
				t.Fatal("invalid login created a refresh session")
			}
		})
	}
}

func TestAuthenticationServicePendingLoginRequestsVerification(t *testing.T) {
	repository := newFakeAuthenticationRepository()
	repository.credential = credentialUser(models.UserStatusPending)
	service := newTestAuthenticationService(t, repository)
	requester := &fakePendingVerificationRequester{}
	service.SetPendingVerificationService(requester)

	_, err := service.Login(context.Background(), LoginCommand{
		Email: "merchant@example.com", Password: "correct-password",
	})
	if !errors.Is(err, ErrEmailVerificationRequired) || requester.userID != testAuthUserID {
		t.Fatalf("error=%v requested user=%q", err, requester.userID)
	}
	if repository.createCalls != 0 {
		t.Fatal("pending login created a refresh session")
	}
}

func TestAuthenticationServicePendingLoginWithWrongPasswordDoesNotRequestVerification(t *testing.T) {
	repository := newFakeAuthenticationRepository()
	repository.credential = credentialUser(models.UserStatusPending)
	service := newTestAuthenticationService(t, repository)
	requester := &fakePendingVerificationRequester{}
	service.SetPendingVerificationService(requester)

	_, err := service.Login(context.Background(), LoginCommand{
		Email: "merchant@example.com", Password: "wrong",
	})
	if !errors.Is(err, ErrInvalidCredentials) || requester.userID != "" {
		t.Fatalf("error=%v requested user=%q", err, requester.userID)
	}
}

func TestAuthenticationServiceRefreshRotatesAndReturnsReplacement(t *testing.T) {
	repository := newFakeAuthenticationRepository()
	repository.rotatedSession = models.RefreshSession{UserID: testAuthUserID}
	service := newTestAuthenticationService(t, repository)

	result, err := service.Refresh(context.Background(), "old-refresh-token")
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if result.RefreshToken == "" || result.RefreshToken == "old-refresh-token" {
		t.Fatalf("replacement refresh token = %q", result.RefreshToken)
	}
	if string(repository.rotation.CurrentTokenHash) == "old-refresh-token" ||
		len(repository.rotation.CurrentTokenHash) != 32 || len(repository.rotation.NewTokenHash) != 32 {
		t.Fatalf("rotation = %#v", repository.rotation)
	}
}

func TestAuthenticationServiceRejectsExpiredAndRevokedRefresh(t *testing.T) {
	for _, state := range []string{"expired", "revoked"} {
		t.Run(state, func(t *testing.T) {
			repository := newFakeAuthenticationRepository()
			repository.rotateErr = models.ErrRefreshTokenInvalid
			service := newTestAuthenticationService(t, repository)
			if _, err := service.Refresh(context.Background(), state+"-token"); !errors.Is(err, ErrInvalidRefresh) {
				t.Fatalf("Refresh() error = %v", err)
			}
		})
	}
	repository := newFakeAuthenticationRepository()
	service := newTestAuthenticationService(t, repository)
	if _, err := service.Refresh(context.Background(), ""); !errors.Is(err, ErrInvalidRefresh) {
		t.Fatalf("empty Refresh() error = %v", err)
	}
}

func TestAuthenticationServiceLogoutAndMe(t *testing.T) {
	repository := newFakeAuthenticationRepository()
	service := newTestAuthenticationService(t, repository)
	if err := service.Logout(context.Background(), "refresh-token"); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if len(repository.revokedHash) != 32 {
		t.Fatalf("revoked hash length = %d", len(repository.revokedHash))
	}
	contextResult, err := service.Me(context.Background(), models.Principal{UserID: testAuthUserID})
	if err != nil || contextResult.User.ID != testAuthUserID {
		t.Fatalf("Me() context=%#v error=%v", contextResult, err)
	}
}

const testAuthUserID = "00000000-0000-4000-8000-000000000001"

func newTestAuthenticationService(t *testing.T, repository *fakeAuthenticationRepository) *AuthenticationService {
	t.Helper()
	service, err := NewAuthenticationService(
		repository,
		&fakePasswordService{},
		&fakeAccessTokenService{},
		30*24*time.Hour,
	)
	if err != nil {
		t.Fatalf("NewAuthenticationService() error = %v", err)
	}
	service.now = func() time.Time { return time.Unix(1800000000, 0).UTC() }
	return service
}

func credentialUser(status models.UserStatus) models.CredentialUser {
	return models.CredentialUser{
		User:         models.User{ID: testAuthUserID, Email: "merchant@example.com", Status: status},
		PasswordHash: "valid-password-hash",
	}
}

type fakePasswordService struct{}

func (*fakePasswordService) Hash(string) (string, error) { return "dummy-password-hash", nil }
func (*fakePasswordService) Verify(hash, password string) error {
	if hash == "valid-password-hash" && password == "correct-password" {
		return nil
	}
	return errors.New("password mismatch")
}

type fakeAccessTokenService struct{}

type fakePendingVerificationRequester struct{ userID string }

func (r *fakePendingVerificationRequester) Request(_ context.Context, userID string) error {
	r.userID = userID
	return nil
}

func (*fakeAccessTokenService) Mint(string) (string, error) { return "access-token", nil }
func (*fakeAccessTokenService) Verify(string) (models.Principal, error) {
	return models.Principal{UserID: testAuthUserID}, nil
}
func (*fakeAccessTokenService) Lifetime() time.Duration { return 15 * time.Minute }

type fakeAuthenticationRepository struct {
	credential     models.CredentialUser
	findErr        error
	email          string
	created        models.RefreshSessionInput
	createCalls    int
	rotation       models.RotationInput
	rotatedSession models.RefreshSession
	rotateErr      error
	revokedHash    []byte
	userContext    models.UserContext
}

func newFakeAuthenticationRepository() *fakeAuthenticationRepository {
	return &fakeAuthenticationRepository{userContext: models.UserContext{
		User: models.User{ID: testAuthUserID, Email: "merchant@example.com", Status: models.UserStatusActive},
		Businesses: []models.BusinessContext{{
			Business:   businessmodels.Business{ID: "00000000-0000-4000-8000-000000000002", Status: businessmodels.StatusActive},
			Membership: businessmodels.Membership{Role: businessmodels.RoleOwner},
		}},
	}}
}

func (r *fakeAuthenticationRepository) FindCredentialUser(_ context.Context, email string) (models.CredentialUser, error) {
	r.email = email
	return r.credential, r.findErr
}
func (r *fakeAuthenticationRepository) CreateRefreshSession(_ context.Context, input models.RefreshSessionInput) (models.RefreshSession, error) {
	r.createCalls++
	r.created = input
	return models.RefreshSession{UserID: input.UserID}, nil
}
func (r *fakeAuthenticationRepository) RotateRefreshSession(_ context.Context, input models.RotationInput) (models.RefreshSession, error) {
	r.rotation = input
	return r.rotatedSession, r.rotateErr
}
func (r *fakeAuthenticationRepository) RevokeRefreshSession(_ context.Context, hash []byte, _ time.Time) (string, error) {
	r.revokedHash = hash
	return testAuthUserID, nil
}
func (r *fakeAuthenticationRepository) GetUserContext(context.Context, string) (models.UserContext, error) {
	return r.userContext, nil
}
