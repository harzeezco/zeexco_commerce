package emailverification

import "time"

const JobTypeEmailVerification = "EMAIL_VERIFICATION"

type DeliveryJob struct {
	ID             string
	VerificationID string
	UserID         string
	Email          string
	FirstName      string
	ExpiresAt      time.Time
	AttemptCount   int
}
