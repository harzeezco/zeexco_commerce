package schemas

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	StatusCode int    `json:"status_code"`
	Message    string `json:"message"`
}

type MeResponse struct {
	User       RegisteredUser          `json:"user"`
	Businesses []AuthenticatedBusiness `json:"businesses"`
}

type SessionResponse struct {
	StatusCode int                     `json:"status_code"`
	Message    string                  `json:"message"`
	User       RegisteredUser          `json:"user"`
	Businesses []AuthenticatedBusiness `json:"businesses"`
}

type AuthenticatedBusiness struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Status         string `json:"status"`
	MembershipRole string `json:"membership_role"`
}
