package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/harzeezco/zeexco_commerce/internal/config"
	"github.com/harzeezco/zeexco_commerce/internal/features/authentication/emailverification"
	"github.com/harzeezco/zeexco_commerce/internal/features/identity"
	"github.com/harzeezco/zeexco_commerce/internal/features/whatsapp/cloudapi"
	whatsappingestion "github.com/harzeezco/zeexco_commerce/internal/features/whatsapp/ingestion"
	whatsapponboarding "github.com/harzeezco/zeexco_commerce/internal/features/whatsapp/onboarding"
	whatsapprepositories "github.com/harzeezco/zeexco_commerce/internal/features/whatsapp/repositories"
	"github.com/harzeezco/zeexco_commerce/internal/platform/database"
	"github.com/jackc/pgx/v5/pgxpool"
)

type WorkerApp struct {
	db            *pgxpool.Pool
	emailWorker   *emailverification.Worker
	webhookWorker *whatsappingestion.Worker
}

func NewWorker(ctx context.Context) (*WorkerApp, error) {
	cfg := config.Load()
	if strings.TrimSpace(cfg.DatabaseURL) == "" {
		return nil, errors.New("DATABASE_URL is required")
	}
	emailRuntime, err := loadEmailVerificationRuntime(cfg)
	if err != nil {
		return nil, fmt.Errorf("configure email worker: %w", err)
	}
	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	emailWorker, err := emailverification.NewWorker(
		emailverification.NewRepository(pool), emailRuntime.sender, emailRuntime.tokens, emailRuntime.worker,
	)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("initialize email worker: %w", err)
	}

	result := &WorkerApp{db: pool, emailWorker: emailWorker}
	webhookEnabled, err := whatsAppWebhookConfiguration(cfg)
	if err != nil {
		pool.Close()
		return nil, err
	}
	if webhookEnabled {
		result.webhookWorker, err = newWhatsAppWorker(pool, cfg)
		if err != nil {
			pool.Close()
			return nil, err
		}
	}
	emailWorker.Start(ctx)
	if result.webhookWorker != nil {
		result.webhookWorker.Start(ctx)
	}
	return result, nil
}

func newWhatsAppWorker(pool *pgxpool.Pool, cfg config.Config) (*whatsappingestion.Worker, error) {
	identityRepository := whatsapprepositories.NewWhatsAppIdentityRepository(pool)
	resolver := identity.NewResolver(map[identity.Provider]identity.ProviderResolver{
		identity.ProviderWhatsApp: identityRepository,
	})
	service, err := whatsappingestion.NewService(
		whatsapprepositories.NewWhatsAppMessageRepository(pool), resolver, nil,
	)
	if err != nil {
		return nil, fmt.Errorf("initialize WhatsApp ingestion worker service: %w", err)
	}
	service.SetWebhookEventRepository(whatsapprepositories.NewWhatsAppWebhookEventRepository(pool))
	if strings.TrimSpace(cfg.PublicFrontendBaseURL) != "" {
		tokenTTL, err := time.ParseDuration(cfg.WhatsAppOnboardingTokenTTL)
		if err != nil || tokenTTL <= 0 {
			return nil, errors.New("WHATSAPP_ONBOARDING_TOKEN_TTL must be a positive duration")
		}
		outbound, err := cloudapi.NewClient(cloudapi.ClientConfig{
			AccessToken: cfg.WhatsAppAccessToken, PhoneNumberID: cfg.WhatsAppPhoneNumberID,
			APIVersion: cfg.MetaGraphAPIVersion, FlowMode: cfg.WhatsAppFlowMode, Logger: log.Default(),
		})
		if err != nil {
			return nil, fmt.Errorf("configure WhatsApp worker outbound client: %w", err)
		}
		onboarding, err := whatsapponboarding.NewService(
			whatsapponboarding.NewRepository(pool), outbound,
			whatsapponboarding.ServiceConfig{
				PublicBaseURL: cfg.PublicFrontendBaseURL, ReturnURL: cfg.WhatsAppOnboardingReturnURL,
				TokenTTL: tokenTTL, MerchantRegistrationFlowID: cfg.WhatsAppMerchantRegistrationFlowID,
				Logger: log.Default(),
			},
		)
		if err != nil {
			return nil, fmt.Errorf("initialize WhatsApp worker onboarding: %w", err)
		}
		service.SetIdentityOutcomeHandler(onboarding)
	}
	poll, err := positiveDuration("WHATSAPP_WEBHOOK_WORKER_POLL_INTERVAL", cfg.WhatsAppWebhookWorkerPollInterval)
	if err != nil {
		return nil, err
	}
	retry, err := positiveDuration("WHATSAPP_WEBHOOK_RETRY_DELAY", cfg.WhatsAppWebhookRetryDelay)
	if err != nil {
		return nil, err
	}
	lease, err := positiveDuration("WHATSAPP_WEBHOOK_PROCESSING_LEASE", cfg.WhatsAppWebhookProcessingLease)
	if err != nil {
		return nil, err
	}
	maxAttempts, err := positiveInteger("WHATSAPP_WEBHOOK_MAX_ATTEMPTS", cfg.WhatsAppWebhookMaxAttempts)
	if err != nil {
		return nil, err
	}
	return whatsappingestion.NewWorker(service, whatsappingestion.WorkerConfig{
		PollInterval: poll, RetryDelay: retry, ProcessingLease: lease,
		MaxAttempts: maxAttempts, Logger: log.Default(),
	})
}

func (a *WorkerApp) Close() {
	if a == nil {
		return
	}
	if a.emailWorker != nil {
		a.emailWorker.Close()
	}
	if a.webhookWorker != nil {
		a.webhookWorker.Close()
	}
	if a.db != nil {
		a.db.Close()
	}
}
