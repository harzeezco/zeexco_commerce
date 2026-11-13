package schemas

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type AuthenticationResponse struct {
	AccessToken string                  `json:"access_token"`
	TokenType   string                  `json:"token_type"`
	ExpiresIn   int64                   `json:"expires_in"`
	User        RegisteredUser          `json:"user"`
	Businesses  []AuthenticatedBusiness `json:"businesses"`
}

type MeResponse struct {
	User       RegisteredUser          `json:"user"`
	Businesses []AuthenticatedBusiness `json:"businesses"`
}

type AuthenticatedBusiness struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Status         string `json:"status"`
	MembershipRole string `json:"membership_role"`
}
