package emailverification

import (
	"context"
	"errors"
	"log"
	"net/url"
	"sync"
	"time"
)

type JobRepository interface {
	ClaimNext(context.Context, time.Time, time.Time, time.Time) (DeliveryJob, bool, error)
	MarkSent(context.Context, string, string, time.Time) error
	MarkFailed(context.Context, string, time.Time, time.Time, string) error
}

type EmailSender interface {
	Send(context.Context, string, EmailPurpose, string, string, string, string) (string, error)
}

type WorkerConfig struct {
	PollInterval    time.Duration
	RetryBaseDelay  time.Duration
	ProcessingLease time.Duration
	VerificationTTL time.Duration
	FrontendURL     string
	Logger          *log.Logger
}

type Worker struct {
	repository JobRepository
	sender     EmailSender
	tokens     *TokenManager
	config     WorkerConfig
	welcomeURL string
	cancel     context.CancelFunc
	wake       chan struct{}
	wg         sync.WaitGroup
}

func NewWorker(repository JobRepository, sender EmailSender, tokens *TokenManager, cfg WorkerConfig) (*Worker, error) {
	if repository == nil || sender == nil || tokens == nil || cfg.PollInterval <= 0 ||
		cfg.RetryBaseDelay <= 0 || cfg.ProcessingLease <= 0 || cfg.VerificationTTL <= 0 {
		return nil, errors.New("email worker dependencies and positive durations are required")
	}
	parsed, err := url.Parse(cfg.FrontendURL)
	if err != nil || !allowedFrontendURL(parsed) {
		return nil, errors.New("EMAIL_VERIFICATION_FRONTEND_URL must be an absolute HTTPS URL, or HTTP localhost URL, without credentials or fragment")
	}
	welcomeURL := *parsed
	welcomeURL.Path = "/login"
	welcomeURL.RawPath = ""
	welcomeURL.RawQuery = ""
	if cfg.Logger == nil {
		cfg.Logger = log.Default()
	}
	return &Worker{
		repository: repository, sender: sender, tokens: tokens,
		config: cfg, welcomeURL: welcomeURL.String(), wake: make(chan struct{}, 1),
	}, nil
}

func (w *Worker) Start(parent context.Context) {
	if w == nil || w.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	w.cancel = cancel
	w.wg.Add(1)
	go w.run(ctx)
}

func (w *Worker) Notify() {
	if w == nil {
		return
	}
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

func (w *Worker) Close() {
	if w == nil || w.cancel == nil {
		return
	}
	w.cancel()
	w.wg.Wait()
}

func (w *Worker) run(ctx context.Context) {
	defer w.wg.Done()
	ticker := time.NewTicker(w.config.PollInterval)
	defer ticker.Stop()
	for {
		w.drain(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-w.wake:
		}
	}
}

func (w *Worker) drain(ctx context.Context) {
	for ctx.Err() == nil {
		now := time.Now().UTC()
		job, claimed, err := w.repository.ClaimNext(
			ctx, now, now.Add(-w.config.ProcessingLease), now.Add(w.config.VerificationTTL),
		)
		if err != nil {
			w.config.Logger.Printf("email worker outcome=claim_failed")
			return
		}
		if !claimed {
			return
		}
		if err := w.deliver(ctx, job, now); err != nil {
			delay := retryDelay(w.config.RetryBaseDelay, job.AttemptCount)
			_ = w.repository.MarkFailed(
				context.Background(), job.ID, time.Now().UTC(), time.Now().UTC().Add(delay), err.Error(),
			)
			w.config.Logger.Printf("email worker outcome=delivery_failed type=%s attempt=%d", job.JobType, job.AttemptCount)
			continue
		}
	}
}

func (w *Worker) deliver(ctx context.Context, job DeliveryJob, now time.Time) error {
	email, err := w.render(job)
	if err != nil {
		return err
	}
	providerID, err := w.sender.Send(
		ctx, job.ID, email.Purpose, job.Email, email.Subject, email.HTMLBody, email.TextBody,
	)
	if err != nil {
		return err
	}
	return w.repository.MarkSent(ctx, job.ID, providerID, now)
}

func (w *Worker) render(job DeliveryJob) (RenderedEmail, error) {
	switch job.JobType {
	case JobTypeEmailVerification:
		token, err := w.tokens.Mint(job.VerificationID, job.ExpiresAt)
		if err != nil {
			return RenderedEmail{}, err
		}
		link, err := verificationLink(w.config.FrontendURL, token)
		if err != nil {
			return RenderedEmail{}, err
		}
		return renderVerificationEmail(job.FirstName, link)
	case JobTypeWelcome:
		return renderWelcomeEmail(job.FirstName, w.welcomeURL)
	default:
		return RenderedEmail{}, errors.New("unsupported email job type")
	}
}

func allowedFrontendURL(parsed *url.URL) bool {
	if parsed == nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return false
	}
	if parsed.Scheme == "https" {
		return true
	}
	host := parsed.Hostname()
	return parsed.Scheme == "http" && (host == "localhost" || host == "127.0.0.1" || host == "::1")
}

func verificationLink(frontendURL, token string) (string, error) {
	parsed, err := url.Parse(frontendURL)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	query.Set("token", token)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func retryDelay(base time.Duration, attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := base
	for count := 1; count < attempt && delay < time.Hour; count++ {
		delay *= 2
	}
	if delay > time.Hour {
		return time.Hour
	}
	return delay
}
