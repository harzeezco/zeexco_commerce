package onboarding

type WhatsAppRegisterRequest struct {
	FirstName       string `json:"first_name"`
	LastName        string `json:"last_name"`
	BusinessName    string `json:"business_name"`
	Email           string `json:"email,omitempty"`
	OnboardingToken string `json:"onboarding_token"`
}
