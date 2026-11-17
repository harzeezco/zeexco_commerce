package authorization

import (
	"context"
	"errors"

	authmodels "github.com/harzeezco/zeexco_commerce/internal/features/authentication/models"
	businessmodels "github.com/harzeezco/zeexco_commerce/internal/features/business/models"
)

var ErrBusinessAccessDenied = errors.New("business access is denied")

type MembershipRepository interface {
	FindBusinessMembership(context.Context, string, string) (businessmodels.Membership, error)
}

type Service struct {
	repository MembershipRepository
}

func NewService(repository MembershipRepository) *Service {
	return &Service{repository: repository}
}

func (s *Service) Authorize(
	ctx context.Context,
	principal authmodels.Principal,
	businessID string,
) (businessmodels.Membership, error) {
	if !authmodels.IsUUID(principal.UserID) || !authmodels.IsUUID(businessID) || s.repository == nil {
		return businessmodels.Membership{}, ErrBusinessAccessDenied
	}
	membership, err := s.repository.FindBusinessMembership(ctx, principal.UserID, businessID)
	if errors.Is(err, authmodels.ErrMembershipNotFound) {
		return businessmodels.Membership{}, ErrBusinessAccessDenied
	}
	if err != nil {
		return businessmodels.Membership{}, err
	}
	return membership, nil
}
