package onboarding

import (
	"errors"
	"time"

	authmodels "github.com/harzeezco/zeexco_commerce/internal/features/authentication/models"
)

const (
	ProviderMetaWhatsApp = "META_WHATSAPP"

	StatusRegistrationPending = "REGISTRATION_PENDING"
	StatusFirstProductPending = "FIRST_PRODUCT_PENDING"
	StatusExpired             = "EXPIRED"

	CreateFirstProductAction = "CREATE_FIRST_PRODUCT"
)

var (
	ErrInvalidSession   = errors.New("onboarding session is invalid")
	ErrMalformedToken   = errors.New("onboarding token is malformed")
	ErrSessionExpired   = errors.New("onboarding session has expired")
	ErrSessionConsumed  = errors.New("onboarding session has already been completed")
	ErrIdentityConflict = errors.New("WhatsApp identity is already linked")
)

type Session struct {
	ID                          string
	Provider                    string
	ProviderUserID              string
	BusinessPhoneNumberID       string
	Status                      string
	ExpiresAt                   time.Time
	WelcomeDeliveryStartedAt    *time.Time
	WelcomeSentAt               *time.Time
	CompletedAt                 *time.Time
	CompletionDeliveryStartedAt *time.Time
	CompletionSentAt            *time.Time
	LinkedUserID                *string
	LinkedBusinessID            *string
	CreatedAt                   time.Time
	UpdatedAt                   time.Time
}

type WelcomePreparation struct {
	Session Session
	Send    bool
}

type Completion struct {
	Session      Session
	Registration authmodels.Registration
}
