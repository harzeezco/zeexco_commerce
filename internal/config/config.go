package config

import "os"

type Config struct {
	AppEnv                             string
	Port                               string
	DatabaseURL                        string
	WhatsAppWebhookVerifyToken         string
	MetaAppSecret                      string
	WhatsAppAccessToken                string
	WhatsAppPhoneNumberID              string
	MetaGraphAPIVersion                string
	PublicFrontendBaseURL              string
	WhatsAppOnboardingReturnURL        string
	WhatsAppOnboardingTokenTTL         string
	WhatsAppFlowPrivateKey             string
	WhatsAppFlowKeyPassphrase          string
	WhatsAppMerchantRegistrationFlowID string
	WhatsAppFlowMode                   string
	AuthJWTPrivateKeyBase64            string
	AuthJWTPublicKeyBase64             string
	AuthJWTIssuer                      string
	AuthJWTAudience                    string
	AuthAccessTokenTTL                 string
	AuthRefreshTokenTTL                string
	AuthRefreshCookieName              string
	AuthRefreshCookieDomain            string
	AuthRefreshCookieSecure            string
	AuthRefreshCookieSameSite          string
	HTTPReadHeaderTimeout              string
	HTTPReadTimeout                    string
	HTTPWriteTimeout                   string
	HTTPIdleTimeout                    string
	HTTPShutdownTimeout                string
	RateLimitWindow                    string
	RateLimitRegister                  string
	RateLimitLogin                     string
	RateLimitRefresh                   string
	RateLimitOnboardingValidate        string
	RateLimitWebhook                   string
	WhatsAppWebhookWorkerPollInterval  string
	WhatsAppWebhookRetryDelay          string
	WhatsAppWebhookProcessingLease     string
	WhatsAppWebhookMaxAttempts         string
	ResendAPIKey                       string
	ResendSecurityFromEmail            string
	ResendLifecycleFromEmail           string
	ResendAPIBaseURL                   string
	EmailVerificationFrontendURL       string
	EmailVerificationHMACSecretBase64  string
	EmailVerificationTTL               string
	EmailWorkerPollInterval            string
	EmailWorkerRetryBaseDelay          string
	EmailWorkerProcessingLease         string
}

func Load() Config {
	return Config{
		AppEnv:                             getEnv("APP_ENV", "development"),
		Port:                               getEnv("PORT", "8080"),
		DatabaseURL:                        os.Getenv("DATABASE_URL"),
		WhatsAppWebhookVerifyToken:         os.Getenv("WHATSAPP_WEBHOOK_VERIFY_TOKEN"),
		MetaAppSecret:                      os.Getenv("META_APP_SECRET"),
		WhatsAppAccessToken:                os.Getenv("WHATSAPP_ACCESS_TOKEN"),
		WhatsAppPhoneNumberID:              os.Getenv("WHATSAPP_PHONE_NUMBER_ID"),
		MetaGraphAPIVersion:                os.Getenv("META_GRAPH_API_VERSION"),
		PublicFrontendBaseURL:              firstEnv("PUBLIC_FRONTEND_BASE_URL", "WHATSAPP_ONBOARDING_PUBLIC_BASE_URL"),
		WhatsAppOnboardingReturnURL:        getEnv("WHATSAPP_ONBOARDING_RETURN_URL", "https://wa.me/"),
		WhatsAppOnboardingTokenTTL:         getEnv("WHATSAPP_ONBOARDING_TOKEN_TTL", "30m"),
		WhatsAppFlowPrivateKey:             os.Getenv("WHATSAPP_FLOW_PRIVATE_KEY"),
		WhatsAppFlowKeyPassphrase:          os.Getenv("WHATSAPP_FLOW_PRIVATE_KEY_PASSPHRASE"),
		WhatsAppMerchantRegistrationFlowID: os.Getenv("WHATSAPP_MERCHANT_REGISTRATION_FLOW_ID"),
		WhatsAppFlowMode:                   getEnv("WHATSAPP_FLOW_MODE", "published"),
		AuthJWTPrivateKeyBase64:            os.Getenv("AUTH_JWT_PRIVATE_KEY_BASE64"),
		AuthJWTPublicKeyBase64:             os.Getenv("AUTH_JWT_PUBLIC_KEY_BASE64"),
		AuthJWTIssuer:                      getEnv("AUTH_JWT_ISSUER", "zeexco"),
		AuthJWTAudience:                    getEnv("AUTH_JWT_AUDIENCE", "zeexco-api"),
		AuthAccessTokenTTL:                 getEnv("AUTH_ACCESS_TOKEN_TTL", "15m"),
		AuthRefreshTokenTTL:                getEnv("AUTH_REFRESH_TOKEN_TTL", "720h"),
		AuthRefreshCookieName:              getEnv("AUTH_REFRESH_COOKIE_NAME", "zeexco_refresh"),
		AuthRefreshCookieDomain:            os.Getenv("AUTH_REFRESH_COOKIE_DOMAIN"),
		AuthRefreshCookieSecure:            getEnv("AUTH_REFRESH_COOKIE_SECURE", "true"),
		AuthRefreshCookieSameSite:          getEnv("AUTH_REFRESH_COOKIE_SAME_SITE", "Lax"),
		HTTPReadHeaderTimeout:              getEnv("HTTP_READ_HEADER_TIMEOUT", "5s"),
		HTTPReadTimeout:                    getEnv("HTTP_READ_TIMEOUT", "15s"),
		HTTPWriteTimeout:                   getEnv("HTTP_WRITE_TIMEOUT", "30s"),
		HTTPIdleTimeout:                    getEnv("HTTP_IDLE_TIMEOUT", "60s"),
		HTTPShutdownTimeout:                getEnv("HTTP_SHUTDOWN_TIMEOUT", "10s"),
		RateLimitWindow:                    getEnv("RATE_LIMIT_WINDOW", "1m"),
		RateLimitRegister:                  getEnv("RATE_LIMIT_REGISTER", "5"),
		RateLimitLogin:                     getEnv("RATE_LIMIT_LOGIN", "10"),
		RateLimitRefresh:                   getEnv("RATE_LIMIT_REFRESH", "20"),
		RateLimitOnboardingValidate:        getEnv("RATE_LIMIT_ONBOARDING_VALIDATE", "30"),
		RateLimitWebhook:                   getEnv("RATE_LIMIT_WEBHOOK", "120"),
		WhatsAppWebhookWorkerPollInterval:  getEnv("WHATSAPP_WEBHOOK_WORKER_POLL_INTERVAL", "250ms"),
		WhatsAppWebhookRetryDelay:          getEnv("WHATSAPP_WEBHOOK_RETRY_DELAY", "5s"),
		WhatsAppWebhookProcessingLease:     getEnv("WHATSAPP_WEBHOOK_PROCESSING_LEASE", "5m"),
		WhatsAppWebhookMaxAttempts:         getEnv("WHATSAPP_WEBHOOK_MAX_ATTEMPTS", "5"),
		ResendAPIKey:                       os.Getenv("RESEND_API_KEY"),
		ResendSecurityFromEmail:            os.Getenv("RESEND_SECURITY_FROM_EMAIL"),
		ResendLifecycleFromEmail:           os.Getenv("RESEND_LIFECYCLE_FROM_EMAIL"),
		ResendAPIBaseURL:                   getEnv("RESEND_API_BASE_URL", "https://api.resend.com"),
		EmailVerificationFrontendURL:       os.Getenv("EMAIL_VERIFICATION_FRONTEND_URL"),
		EmailVerificationHMACSecretBase64:  os.Getenv("EMAIL_VERIFICATION_HMAC_SECRET_BASE64"),
		EmailVerificationTTL:               getEnv("EMAIL_VERIFICATION_TTL", "24h"),
		EmailWorkerPollInterval:            getEnv("EMAIL_WORKER_POLL_INTERVAL", "250ms"),
		EmailWorkerRetryBaseDelay:          getEnv("EMAIL_WORKER_RETRY_BASE_DELAY", "30s"),
		EmailWorkerProcessingLease:         getEnv("EMAIL_WORKER_PROCESSING_LEASE", "5m"),
	}
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if value := os.Getenv(key); value != "" {
			return value
		}
	}
	return ""
}

func getEnv(key, fallback string) string {
	value := os.Getenv(key)

	if value == "" {
		return fallback
	}

	return value
}
