package authorization

import (
	"context"
	"errors"
	"testing"

	authmodels "github.com/harzeezco/zeexco_commerce/internal/features/authentication/models"
	businessmodels "github.com/harzeezco/zeexco_commerce/internal/features/business/models"
)

func TestServiceAuthorizesMembership(t *testing.T) {
	repository := &fakeMembershipRepository{membership: businessmodels.Membership{
		UserID: testAuthorizationUserID, BusinessID: testAuthorizationBusinessID,
		Role: businessmodels.RoleOwner,
	}}
	membership, err := NewService(repository).Authorize(
		context.Background(),
		authmodels.Principal{UserID: testAuthorizationUserID},
		testAuthorizationBusinessID,
	)
	if err != nil || membership.Role != businessmodels.RoleOwner {
		t.Fatalf("membership=%#v error=%v", membership, err)
	}
}

func TestServiceDeniesAnotherBusinessAndInvalidIDs(t *testing.T) {
	repository := &fakeMembershipRepository{err: authmodels.ErrMembershipNotFound}
	service := NewService(repository)
	for _, test := range []struct {
		principal authmodels.Principal
		business  string
	}{
		{principal: authmodels.Principal{UserID: testAuthorizationUserID}, business: testAuthorizationBusinessID},
		{principal: authmodels.Principal{UserID: "invalid"}, business: testAuthorizationBusinessID},
		{principal: authmodels.Principal{UserID: testAuthorizationUserID}, business: "invalid"},
	} {
		if _, err := service.Authorize(context.Background(), test.principal, test.business); !errors.Is(err, ErrBusinessAccessDenied) {
			t.Fatalf("Authorize() error=%v", err)
		}
	}
}

const (
	testAuthorizationUserID     = "00000000-0000-4000-8000-000000000001"
	testAuthorizationBusinessID = "00000000-0000-4000-8000-000000000002"
)

type fakeMembershipRepository struct {
	membership businessmodels.Membership
	err        error
}

func (r *fakeMembershipRepository) FindBusinessMembership(context.Context, string, string) (businessmodels.Membership, error) {
	return r.membership, r.err
}
