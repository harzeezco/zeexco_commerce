package emailverification

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type recordingJobRepository struct {
	sentID string
	failed int
}

func (*recordingJobRepository) ClaimNext(context.Context, time.Time, time.Time, time.Time) (DeliveryJob, bool, error) {
	return DeliveryJob{}, false, nil
}

func (r *recordingJobRepository) MarkSent(_ context.Context, _ string, providerID string, _ time.Time) error {
	r.sentID = providerID
	return nil
}

func (r *recordingJobRepository) MarkFailed(context.Context, string, time.Time, time.Time, string) error {
	r.failed++
	return nil
}

type oneJobRepository struct {
	job                DeliveryJob
	claimed            bool
	failedAt, retryAt  time.Time
	failureDescription string
}

func (r *oneJobRepository) ClaimNext(context.Context, time.Time, time.Time, time.Time) (DeliveryJob, bool, error) {
	if r.claimed {
		return DeliveryJob{}, false, nil
	}
	r.claimed = true
	return r.job, true, nil
}

func (*oneJobRepository) MarkSent(context.Context, string, string, time.Time) error { return nil }

func (r *oneJobRepository) MarkFailed(_ context.Context, _ string, failedAt, retryAt time.Time, message string) error {
	r.failedAt, r.retryAt, r.failureDescription = failedAt, retryAt, message
	return nil
}

type recordedEmail struct {
	jobID, to, subject, html, text string
	purpose                        EmailPurpose
}

type recordingSender struct {
	emails []recordedEmail
	err    error
}

func (s *recordingSender) Send(
	_ context.Context,
	jobID string,
	purpose EmailPurpose,
	to, subject, htmlBody, textBody string,
) (string, error) {
	s.emails = append(s.emails, recordedEmail{
		jobID: jobID, purpose: purpose, to: to, subject: subject, html: htmlBody, text: textBody,
	})
	if s.err != nil {
		return "", s.err
	}
	return "email_123", nil
}

func newTestWorker(t *testing.T, repository JobRepository, sender EmailSender) *Worker {
	t.Helper()
	tokens, err := NewTokenManager([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	worker, err := NewWorker(repository, sender, tokens, WorkerConfig{
		PollInterval: time.Second, RetryBaseDelay: time.Second, ProcessingLease: time.Minute,
		VerificationTTL: time.Hour, FrontendURL: "http://localhost:3000/verify-email",
	})
	if err != nil {
		t.Fatal(err)
	}
	return worker
}

func TestWorkerRendersVerificationEmailForSecuritySender(t *testing.T) {
	repository := &recordingJobRepository{}
	sender := &recordingSender{}
	worker := newTestWorker(t, repository, sender)
	now := time.Now().UTC()
	err := worker.deliver(context.Background(), DeliveryJob{
		ID: "job-1", JobType: JobTypeEmailVerification,
		VerificationID: "00000000-0000-4000-8000-000000000001",
		Email:          "ada@example.com", FirstName: "Ada", ExpiresAt: now.Add(time.Hour),
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(sender.emails) != 1 {
		t.Fatalf("sent emails=%d", len(sender.emails))
	}
	email := sender.emails[0]
	if email.purpose != EmailPurposeSecurity || email.jobID != "job-1" || email.to != "ada@example.com" ||
		!strings.Contains(email.html, "http://localhost:3000/verify-email?token=") ||
		!strings.Contains(email.text, "Confirm your email address") || repository.sentID != "email_123" {
		t.Fatalf("email=%#v repository=%#v", email, repository)
	}
}

func TestWorkerRendersWelcomeEmailForLifecycleSender(t *testing.T) {
	repository := &recordingJobRepository{}
	sender := &recordingSender{}
	worker := newTestWorker(t, repository, sender)
	now := time.Now().UTC()
	err := worker.deliver(context.Background(), DeliveryJob{
		ID: "job-2", JobType: JobTypeWelcome, Email: "ada@example.com", FirstName: "Ada",
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	email := sender.emails[0]
	if email.purpose != EmailPurposeLifecycle || email.subject != "Welcome to Zeexco" ||
		!strings.Contains(email.html, "http://localhost:3000/login") ||
		!strings.Contains(email.text, "Your account is ready") || repository.sentID != "email_123" {
		t.Fatalf("email=%#v repository=%#v", email, repository)
	}
}

func TestWorkerRetryAfterCrashReusesStableJobIdentity(t *testing.T) {
	repository := &recordingJobRepository{}
	sender := &recordingSender{}
	worker := newTestWorker(t, repository, sender)
	job := DeliveryJob{ID: "stable-job", JobType: JobTypeWelcome, Email: "ada@example.com", FirstName: "Ada"}
	if err := worker.deliver(context.Background(), job, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	// A crash after the provider accepted the first request but before its SENT update
	// causes the durable lease to be reclaimed. Resend sees the same job identity again.
	if err := worker.deliver(context.Background(), job, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if len(sender.emails) != 2 || sender.emails[0].jobID != "stable-job" || sender.emails[1].jobID != "stable-job" {
		t.Fatalf("emails=%#v", sender.emails)
	}
}

func TestWorkerReturnsProviderFailureWithoutMarkingSent(t *testing.T) {
	repository := &recordingJobRepository{}
	sender := &recordingSender{err: errors.New("resend unavailable")}
	worker := newTestWorker(t, repository, sender)
	err := worker.deliver(context.Background(), DeliveryJob{
		ID: "job-3", JobType: JobTypeWelcome, Email: "ada@example.com", FirstName: "Ada",
	}, time.Now().UTC())
	if err == nil || repository.sentID != "" {
		t.Fatalf("error=%v repository=%#v", err, repository)
	}
}

func TestWorkerFailedWelcomeDeliveryIsScheduledForRetry(t *testing.T) {
	repository := &oneJobRepository{job: DeliveryJob{
		ID: "job-4", JobType: JobTypeWelcome, Email: "ada@example.com", FirstName: "Ada", AttemptCount: 1,
	}}
	sender := &recordingSender{err: errors.New("resend unavailable")}
	worker := newTestWorker(t, repository, sender)
	worker.drain(context.Background())
	if repository.failureDescription != "resend unavailable" || repository.failedAt.IsZero() ||
		!repository.retryAt.Equal(repository.failedAt.Add(time.Second)) {
		t.Fatalf("repository=%#v", repository)
	}
}
