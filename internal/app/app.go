package app

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/harzeezco/zeexco_commerce/internal/config"
	"github.com/harzeezco/zeexco_commerce/internal/features/authentication/controllers"
	"github.com/harzeezco/zeexco_commerce/internal/features/authentication/emailverification"
	authmiddleware "github.com/harzeezco/zeexco_commerce/internal/features/authentication/middleware"
	authrepositories "github.com/harzeezco/zeexco_commerce/internal/features/authentication/repositories"
	"github.com/harzeezco/zeexco_commerce/internal/features/authentication/routes"
	"github.com/harzeezco/zeexco_commerce/internal/features/authentication/security"
	"github.com/harzeezco/zeexco_commerce/internal/features/authentication/services"
	"github.com/harzeezco/zeexco_commerce/internal/features/identity"
	"github.com/harzeezco/zeexco_commerce/internal/features/whatsapp/cloudapi"
	whatsappcontrollers "github.com/harzeezco/zeexco_commerce/internal/features/whatsapp/controllers"
	whatsappflows "github.com/harzeezco/zeexco_commerce/internal/features/whatsapp/flows"
	whatsappingestion "github.com/harzeezco/zeexco_commerce/internal/features/whatsapp/ingestion"
	whatsapponboarding "github.com/harzeezco/zeexco_commerce/internal/features/whatsapp/onboarding"
	whatsapprepositories "github.com/harzeezco/zeexco_commerce/internal/features/whatsapp/repositories"
	whatsapproutes "github.com/harzeezco/zeexco_commerce/internal/features/whatsapp/routes"
	"github.com/harzeezco/zeexco_commerce/internal/platform/database"
	"github.com/harzeezco/zeexco_commerce/internal/platform/httpapi"
	"github.com/jackc/pgx/v5/pgxpool"
)

type authenticationRuntime struct {
	tokens     *security.AccessTokenManager
	refreshTTL time.Duration
	cookie     controllers.RefreshCookieConfig
}

type App struct {
	handler http.Handler
	db      *pgxpool.Pool
}

func New(ctx context.Context) (*App, error) {
	cfg := config.Load()

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	whatsAppWebhookEnabled, err := whatsAppWebhookConfiguration(cfg)
	if err != nil {
		return nil, err
	}
	whatsAppFlowEnabled, err := whatsAppFlowConfiguration(cfg)
	if err != nil {
		return nil, err
	}
	authRuntime, err := loadAuthenticationRuntime(cfg)
	if err != nil {
		return nil, fmt.Errorf("configure authentication: %w", err)
	}
	emailRuntime, err := loadEmailVerificationCore(cfg)
	if err != nil {
		return nil, fmt.Errorf("configure email verification: %w", err)
	}

	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	migrationReport, err := database.Migrate(ctx, pool)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("migrate database: %w", err)
	}
	if len(migrationReport.Applied) > 0 || len(migrationReport.Baselined) > 0 {
		log.Printf(
			"database migrations complete applied=%d baselined=%d",
			len(migrationReport.Applied), len(migrationReport.Baselined),
		)
	}
	if migrationReport.LegacyActiveUsersWithoutPassword > 0 {
		log.Printf(
			"warning: %d legacy ACTIVE user(s) have no password hash and require explicit account remediation",
			migrationReport.LegacyActiveUsersWithoutPassword,
		)
	}

	mux := http.NewServeMux()
	metrics := httpapi.NewMetrics()
	rateLimitWindow, err := positiveDuration("RATE_LIMIT_WINDOW", cfg.RateLimitWindow)
	if err != nil {
		pool.Close()
		return nil, err
	}
	registerLimit, err := positiveInteger("RATE_LIMIT_REGISTER", cfg.RateLimitRegister)
	if err != nil {
		pool.Close()
		return nil, err
	}
	loginLimit, err := positiveInteger("RATE_LIMIT_LOGIN", cfg.RateLimitLogin)
	if err != nil {
		pool.Close()
		return nil, err
	}
	refreshLimit, err := positiveInteger("RATE_LIMIT_REFRESH", cfg.RateLimitRefresh)
	if err != nil {
		pool.Close()
		return nil, err
	}
	onboardingValidateLimit, err := positiveInteger("RATE_LIMIT_ONBOARDING_VALIDATE", cfg.RateLimitOnboardingValidate)
	if err != nil {
		pool.Close()
		return nil, err
	}
	webhookLimit, err := positiveInteger("RATE_LIMIT_WEBHOOK", cfg.RateLimitWebhook)
	if err != nil {
		pool.Close()
		return nil, err
	}
	allowedOrigin, err := normalizeHTTPOrigin(cfg.PublicFrontendBaseURL)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("configure frontend origin: %w", err)
	}
	mux.HandleFunc("GET /health/live", httpapi.Live)
	mux.HandleFunc("GET /health/ready", httpapi.Ready(pool))
	mux.Handle("GET /metrics", metrics.Handler())
	registerLimiter := httpapi.NewLimiter(registerLimit, rateLimitWindow)
	webhookLimiter := httpapi.NewLimiter(webhookLimit, rateLimitWindow)

	passwords := security.NewBcryptPasswordHasher()
	emailRepository := emailverification.NewRepository(pool)
	emailService, err := emailverification.NewService(emailRepository, emailRuntime.tokens, emailRuntime.ttl)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("initialize email verification: %w", err)
	}
	registrationRepository := authrepositories.NewRegistrationRepositoryWithVerificationTTL(pool, emailRuntime.ttl)
	registerService, err := services.NewRegisterService(
		registrationRepository,
		passwords,
	)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("initialize registration: %w", err)
	}

	authController := controllers.NewAuthController(
		registerService,
	)
	authenticationRepository := authrepositories.NewAuthenticationRepository(pool)
	authenticationService, err := services.NewAuthenticationService(
		authenticationRepository,
		passwords,
		authRuntime.tokens,
		authRuntime.refreshTTL,
	)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("initialize authentication: %w", err)
	}
	authenticationService.SetPendingVerificationService(emailService)
	authenticationController := controllers.NewAuthenticationController(
		authenticationService,
		authRuntime.cookie,
	)
	authenticationMiddleware := authmiddleware.NewAuthentication(authRuntime.tokens)

	mux.Handle(routes.RegisterUser, httpapi.RateLimit(
		registerLimiter, "register", metrics,
		http.HandlerFunc(authController.Register),
	))
	mux.Handle(routes.VerifyEmail, httpapi.RateLimit(
		registerLimiter, "verify_email", metrics,
		http.HandlerFunc(emailverification.NewController(emailService).Verify),
	))
	mux.Handle(routes.LoginUser, httpapi.RateLimit(
		httpapi.NewLimiter(loginLimit, rateLimitWindow), "login", metrics,
		http.HandlerFunc(authenticationController.Login),
	))
	mux.Handle(routes.RefreshUser, httpapi.RateLimit(
		httpapi.NewLimiter(refreshLimit, rateLimitWindow), "refresh", metrics,
		httpapi.RequireOrigin(allowedOrigin, http.HandlerFunc(authenticationController.Refresh)),
	))
	mux.Handle(routes.LogoutUser,
		httpapi.RequireOrigin(allowedOrigin, http.HandlerFunc(authenticationController.Logout)),
	)
	mux.Handle(
		routes.CurrentUser,
		authenticationMiddleware.Require(http.HandlerFunc(authenticationController.Me)),
	)

	if whatsAppWebhookEnabled || whatsAppFlowEnabled {
		whatsappIdentityRepository := whatsapprepositories.NewWhatsAppIdentityRepository(pool)
		identityResolver := identity.NewResolver(map[identity.Provider]identity.ProviderResolver{
			identity.ProviderWhatsApp: whatsappIdentityRepository,
		})
		whatsappMessageRepository := whatsapprepositories.NewWhatsAppMessageRepository(pool)
		whatsappWebhookEventRepository := whatsapprepositories.NewWhatsAppWebhookEventRepository(pool)
		whatsappIngestionService, err := whatsappingestion.NewService(
			whatsappMessageRepository,
			identityResolver,
			nil,
		)
		if err != nil {
			pool.Close()
			return nil, fmt.Errorf("initialize WhatsApp ingestion: %w", err)
		}
		whatsappIngestionService.SetWebhookEventRepository(whatsappWebhookEventRepository)
		var onboardingService *whatsapponboarding.Service
		if strings.TrimSpace(cfg.PublicFrontendBaseURL) != "" {
			tokenTTL, err := time.ParseDuration(cfg.WhatsAppOnboardingTokenTTL)
			if err != nil || tokenTTL <= 0 {
				pool.Close()
				return nil, errors.New("WHATSAPP_ONBOARDING_TOKEN_TTL must be a positive duration")
			}
			outboundClient, err := cloudapi.NewClient(cloudapi.ClientConfig{
				AccessToken: cfg.WhatsAppAccessToken, PhoneNumberID: cfg.WhatsAppPhoneNumberID,
				APIVersion: cfg.MetaGraphAPIVersion, FlowMode: cfg.WhatsAppFlowMode, Logger: log.Default(),
			})
			if err != nil {
				pool.Close()
				return nil, fmt.Errorf("configure WhatsApp onboarding outbound client: %w", err)
			}
			onboardingRepository := whatsapponboarding.NewRepository(pool)
			onboardingService, err = whatsapponboarding.NewService(
				onboardingRepository, outboundClient,
				whatsapponboarding.ServiceConfig{
					PublicBaseURL:              cfg.PublicFrontendBaseURL,
					ReturnURL:                  cfg.WhatsAppOnboardingReturnURL,
					TokenTTL:                   tokenTTL,
					MerchantRegistrationFlowID: cfg.WhatsAppMerchantRegistrationFlowID,
					Logger:                     log.Default(),
				},
			)
			if err != nil {
				pool.Close()
				return nil, fmt.Errorf("initialize WhatsApp onboarding: %w", err)
			}
			whatsappIngestionService.SetIdentityOutcomeHandler(onboardingService)
			onboardingController := whatsapponboarding.NewController(onboardingService)
			pageLimiter := httpapi.NewLimiter(onboardingValidateLimit, rateLimitWindow)
			mux.Handle(whatsapproutes.OnboardingPage, httpapi.RateLimit(
				pageLimiter, "onboarding_validate", metrics, http.HandlerFunc(onboardingController.Page),
			))
			mux.Handle(whatsapproutes.OnboardingSubmit, httpapi.RateLimit(
				registerLimiter, "register", metrics, http.HandlerFunc(onboardingController.Submit),
			))
			mux.Handle(whatsapproutes.OnboardingValidate, httpapi.RateLimit(
				pageLimiter, "onboarding_validate", metrics, http.HandlerFunc(onboardingController.ValidateAPI),
			))
			mux.Handle(whatsapproutes.OnboardingRegister, httpapi.RateLimit(
				registerLimiter, "register", metrics, http.HandlerFunc(onboardingController.RegisterAPI),
			))
		}
		if whatsAppFlowEnabled {
			flowProtocol, err := whatsappflows.NewProtocol(
				cfg.WhatsAppFlowPrivateKey,
				cfg.WhatsAppFlowKeyPassphrase,
			)
			if err != nil {
				pool.Close()
				return nil, fmt.Errorf("configure WhatsApp Flow encryption: %w", err)
			}
			flowController, err := whatsappflows.NewController(
				flowProtocol,
				whatsappflows.NewActionHandler(onboardingService),
				cfg.MetaAppSecret,
				log.Default(),
			)
			if err != nil {
				pool.Close()
				return nil, fmt.Errorf("configure WhatsApp Flow endpoint: %w", err)
			}
			mux.Handle(whatsapproutes.FlowDataExchange, httpapi.RateLimit(
				webhookLimiter, "whatsapp_webhook", metrics, http.HandlerFunc(flowController.Exchange),
			))
		}

		if whatsAppWebhookEnabled {
			whatsappWebhookController := whatsappcontrollers.NewWebhookController(
				cfg.WhatsAppWebhookVerifyToken,
				cfg.MetaAppSecret,
				whatsappIngestionService,
				log.Default(),
			)
			mux.HandleFunc(whatsapproutes.VerifyWebhook, whatsappWebhookController.Verify)
			mux.Handle(whatsapproutes.ReceiveWebhook, httpapi.RateLimit(
				webhookLimiter, "whatsapp_webhook", metrics,
				http.HandlerFunc(whatsappWebhookController.Receive),
			))
		}
	}

	handler, err := newCORSHandler(mux, cfg.PublicFrontendBaseURL)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("configure frontend CORS: %w", err)
	}
	handler = httpapi.RequestMiddleware(handler, metrics, log.Default())
	return &App{handler: handler, db: pool}, nil

}

type emailVerificationRuntime struct {
	tokens *emailverification.TokenManager
	sender *emailverification.ResendClient
	ttl    time.Duration
	worker emailverification.WorkerConfig
}

type emailVerificationCore struct {
	tokens *emailverification.TokenManager
	ttl    time.Duration
}

func loadEmailVerificationCore(cfg config.Config) (emailVerificationCore, error) {
	secret, err := decodeBase64Key(cfg.EmailVerificationHMACSecretBase64, "EMAIL_VERIFICATION_HMAC_SECRET_BASE64")
	if err != nil {
		return emailVerificationCore{}, err
	}
	tokens, err := emailverification.NewTokenManager(secret)
	if err != nil {
		return emailVerificationCore{}, err
	}
	ttl, err := positiveDuration("EMAIL_VERIFICATION_TTL", cfg.EmailVerificationTTL)
	if err != nil {
		return emailVerificationCore{}, err
	}
	return emailVerificationCore{tokens: tokens, ttl: ttl}, nil
}

func loadEmailVerificationRuntime(cfg config.Config) (emailVerificationRuntime, error) {
	secret, err := decodeBase64Key(cfg.EmailVerificationHMACSecretBase64, "EMAIL_VERIFICATION_HMAC_SECRET_BASE64")
	if err != nil {
		return emailVerificationRuntime{}, err
	}
	tokens, err := emailverification.NewTokenManager(secret)
	if err != nil {
		return emailVerificationRuntime{}, err
	}
	sender, err := emailverification.NewResendClient(emailverification.ResendConfig{
		APIKey: cfg.ResendAPIKey, SecurityFrom: cfg.ResendSecurityFromEmail,
		LifecycleFrom: cfg.ResendLifecycleFromEmail, BaseURL: cfg.ResendAPIBaseURL,
	})
	if err != nil {
		return emailVerificationRuntime{}, err
	}
	ttl, err := positiveDuration("EMAIL_VERIFICATION_TTL", cfg.EmailVerificationTTL)
	if err != nil {
		return emailVerificationRuntime{}, err
	}
	poll, err := positiveDuration("EMAIL_WORKER_POLL_INTERVAL", cfg.EmailWorkerPollInterval)
	if err != nil {
		return emailVerificationRuntime{}, err
	}
	retry, err := positiveDuration("EMAIL_WORKER_RETRY_BASE_DELAY", cfg.EmailWorkerRetryBaseDelay)
	if err != nil {
		return emailVerificationRuntime{}, err
	}
	lease, err := positiveDuration("EMAIL_WORKER_PROCESSING_LEASE", cfg.EmailWorkerProcessingLease)
	if err != nil {
		return emailVerificationRuntime{}, err
	}
	return emailVerificationRuntime{tokens: tokens, sender: sender, ttl: ttl, worker: emailverification.WorkerConfig{
		PollInterval: poll, RetryBaseDelay: retry, ProcessingLease: lease,
		VerificationTTL: ttl, FrontendURL: cfg.EmailVerificationFrontendURL, Logger: log.Default(),
	}}, nil
}

func loadAuthenticationRuntime(cfg config.Config) (authenticationRuntime, error) {
	privateKey, err := decodeBase64Key(cfg.AuthJWTPrivateKeyBase64, "AUTH_JWT_PRIVATE_KEY_BASE64")
	if err != nil {
		return authenticationRuntime{}, err
	}
	publicKey, err := decodeBase64Key(cfg.AuthJWTPublicKeyBase64, "AUTH_JWT_PUBLIC_KEY_BASE64")
	if err != nil {
		return authenticationRuntime{}, err
	}
	accessTTL, err := time.ParseDuration(cfg.AuthAccessTokenTTL)
	if err != nil || accessTTL <= 0 {
		return authenticationRuntime{}, errors.New("AUTH_ACCESS_TOKEN_TTL must be a positive duration")
	}
	refreshTTL, err := time.ParseDuration(cfg.AuthRefreshTokenTTL)
	if err != nil || refreshTTL <= 0 {
		return authenticationRuntime{}, errors.New("AUTH_REFRESH_TOKEN_TTL must be a positive duration")
	}
	secure, err := strconv.ParseBool(cfg.AuthRefreshCookieSecure)
	if err != nil {
		return authenticationRuntime{}, errors.New("AUTH_REFRESH_COOKIE_SECURE must be true or false")
	}
	sameSite, err := parseSameSite(cfg.AuthRefreshCookieSameSite)
	if err != nil {
		return authenticationRuntime{}, err
	}
	if sameSite == http.SameSiteNoneMode && !secure {
		return authenticationRuntime{}, errors.New("SameSite=None refresh cookies must be Secure")
	}
	if strings.TrimSpace(cfg.AuthRefreshCookieName) == "" {
		return authenticationRuntime{}, errors.New("AUTH_REFRESH_COOKIE_NAME is required")
	}
	tokens, err := security.NewAccessTokenManager(
		privateKey, publicKey, cfg.AuthJWTIssuer, cfg.AuthJWTAudience, accessTTL,
	)
	if err != nil {
		return authenticationRuntime{}, err
	}
	return authenticationRuntime{
		tokens:     tokens,
		refreshTTL: refreshTTL,
		cookie: controllers.RefreshCookieConfig{
			Name: strings.TrimSpace(cfg.AuthRefreshCookieName), Path: "/api/v1/auth",
			Domain: cfg.AuthRefreshCookieDomain, Secure: secure,
			SameSite: sameSite, Lifetime: refreshTTL,
		},
	}, nil
}

func decodeBase64Key(value, variable string) ([]byte, error) {
	if strings.TrimSpace(value) == "" {
		return nil, fmt.Errorf("%s is required", variable)
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil {
		return nil, fmt.Errorf("%s must be standard base64: %w", variable, err)
	}
	return decoded, nil
}

func parseSameSite(value string) (http.SameSite, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "lax":
		return http.SameSiteLaxMode, nil
	case "strict":
		return http.SameSiteStrictMode, nil
	case "none":
		return http.SameSiteNoneMode, nil
	default:
		return http.SameSiteDefaultMode, errors.New("AUTH_REFRESH_COOKIE_SAME_SITE must be Lax, Strict, or None")
	}
}

func whatsAppWebhookConfiguration(cfg config.Config) (bool, error) {
	verifyTokenConfigured := strings.TrimSpace(cfg.WhatsAppWebhookVerifyToken) != ""
	metaSecretConfigured := strings.TrimSpace(cfg.MetaAppSecret) != ""
	flowKeyConfigured := strings.TrimSpace(cfg.WhatsAppFlowPrivateKey) != ""
	if verifyTokenConfigured && !metaSecretConfigured {
		return false, fmt.Errorf(
			"WHATSAPP_WEBHOOK_VERIFY_TOKEN and META_APP_SECRET must both be configured to enable WhatsApp webhooks",
		)
	}
	if metaSecretConfigured && !verifyTokenConfigured && !flowKeyConfigured {
		return false, fmt.Errorf(
			"WHATSAPP_WEBHOOK_VERIFY_TOKEN and META_APP_SECRET must both be configured to enable WhatsApp webhooks",
		)
	}
	return verifyTokenConfigured, nil
}

func whatsAppFlowConfiguration(cfg config.Config) (bool, error) {
	privateKeyConfigured := strings.TrimSpace(cfg.WhatsAppFlowPrivateKey) != ""
	passphraseConfigured := strings.TrimSpace(cfg.WhatsAppFlowKeyPassphrase) != ""
	if passphraseConfigured && !privateKeyConfigured {
		return false, errors.New("WHATSAPP_FLOW_PRIVATE_KEY_PASSPHRASE requires WHATSAPP_FLOW_PRIVATE_KEY")
	}
	if privateKeyConfigured && strings.TrimSpace(cfg.MetaAppSecret) == "" {
		return false, errors.New("META_APP_SECRET is required when WHATSAPP_FLOW_PRIVATE_KEY is configured")
	}
	return privateKeyConfigured, nil
}

func (a *App) Handler() http.Handler {
	return a.handler
}

func newCORSHandler(next http.Handler, configuredOrigin string) (http.Handler, error) {
	origin, err := normalizeHTTPOrigin(configuredOrigin)
	if err != nil {
		return nil, err
	}
	if origin == "" {
		return next, nil
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestOrigin := r.Header.Get("Origin")
		if requestOrigin != "" && requestOrigin != origin {
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
			return
		}
		if requestOrigin == origin {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Add("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	}), nil
}

func normalizeHTTPOrigin(configuredOrigin string) (string, error) {
	configuredOrigin = strings.TrimSpace(configuredOrigin)
	if configuredOrigin == "" {
		return "", nil
	}
	parsed, err := url.Parse(configuredOrigin)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
		(parsed.Scheme != "https" && parsed.Scheme != "http") || (parsed.Path != "" && parsed.Path != "/") {
		return "", errors.New("PUBLIC_FRONTEND_BASE_URL must be an HTTP(S) origin without path, credentials, query, or fragment")
	}
	return parsed.Scheme + "://" + parsed.Host, nil
}

func (a *App) Close() {
	a.db.Close()
}

func positiveDuration(name, value string) (time.Duration, error) {
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", name)
	}
	return duration, nil
}

func positiveInteger(name, value string) (int, error) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return parsed, nil
}
