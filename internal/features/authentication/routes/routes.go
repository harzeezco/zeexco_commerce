package routes

const (
	RegisterUser = "POST /api/v1/auth/register"
	LoginUser    = "POST /api/v1/auth/login"
	RefreshUser  = "POST /api/v1/auth/refresh"
	LogoutUser   = "POST /api/v1/auth/logout"
	CurrentUser  = "GET /api/v1/auth/me"
	VerifyEmail  = "POST /api/v1/auth/verify-email"
)
