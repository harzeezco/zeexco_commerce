package emailverification

import "time"

const (
	JobTypeEmailVerification = "EMAIL_VERIFICATION"
	JobTypeWelcome           = "WELCOME"
)

type EmailPurpose string

const (
	EmailPurposeSecurity  EmailPurpose = "security"
	EmailPurposeLifecycle EmailPurpose = "lifecycle"
)

type DeliveryJob struct {
	ID             string
	JobType        string
	VerificationID string
	UserID         string
	Email          string
	FirstName      string
	ExpiresAt      time.Time
	AttemptCount   int
}
