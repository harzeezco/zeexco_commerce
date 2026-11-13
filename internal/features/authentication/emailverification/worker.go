package emailverification

import (
	"context"
	"errors"
	"fmt"
	"html"
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
	Send(context.Context, string, string, string, string, string) (string, error)
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
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return nil, errors.New("EMAIL_VERIFICATION_FRONTEND_URL must be an absolute HTTPS URL without credentials or fragment")
	}
	if cfg.Logger == nil {
		cfg.Logger = log.Default()
	}
	return &Worker{
		repository: repository, sender: sender, tokens: tokens,
		config: cfg, wake: make(chan struct{}, 1),
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
			w.config.Logger.Printf("email verification worker outcome=claim_failed")
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
			w.config.Logger.Printf("email verification worker outcome=delivery_failed attempt=%d", job.AttemptCount)
			continue
		}
	}
}

func (w *Worker) deliver(ctx context.Context, job DeliveryJob, now time.Time) error {
	token, err := w.tokens.Mint(job.VerificationID, job.ExpiresAt)
	if err != nil {
		return err
	}
	link, err := verificationLink(w.config.FrontendURL, token)
	if err != nil {
		return err
	}
	name := html.EscapeString(job.FirstName)
	safeLink := html.EscapeString(link)
	htmlBody := fmt.Sprintf(
		`<p>Hello %s,</p><p>Confirm your email address to activate your Zeexco account.</p><p><a href="%s">Verify email address</a></p><p>If the button does not work, copy this address into your browser:</p><p>%s</p>`,
		name, safeLink, safeLink,
	)
	textBody := fmt.Sprintf(
		"Hello %s,\n\nConfirm your email address to activate your Zeexco account:\n%s\n",
		job.FirstName, link,
	)
	providerID, err := w.sender.Send(
		ctx, job.ID, job.Email, "Verify your Zeexco email address", htmlBody, textBody,
	)
	if err != nil {
		return err
	}
	return w.repository.MarkSent(ctx, job.ID, providerID, now)
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
