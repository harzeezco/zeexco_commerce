package emailverification

import (
	"context"
	"strings"
	"testing"
	"time"
)

type recordingJobRepository struct {
	sentID string
}

func (*recordingJobRepository) ClaimNext(context.Context, time.Time, time.Time, time.Time) (DeliveryJob, bool, error) {
	return DeliveryJob{}, false, nil
}
func (r *recordingJobRepository) MarkSent(_ context.Context, _ string, providerID string, _ time.Time) error {
	r.sentID = providerID
	return nil
}
func (*recordingJobRepository) MarkFailed(context.Context, string, time.Time, time.Time, string) error {
	return nil
}

type recordingSender struct {
	to, html, idempotencyKey string
}

func (s *recordingSender) Send(_ context.Context, key, to, _ string, html, _ string) (string, error) {
	s.to, s.html, s.idempotencyKey = to, html, key
	return "email_123", nil
}

func TestWorkerBuildsActivationLinkAndMarksSent(t *testing.T) {
	repository := &recordingJobRepository{}
	sender := &recordingSender{}
	tokens, _ := NewTokenManager([]byte("0123456789abcdef0123456789abcdef"))
	now := time.Now().UTC()
	worker, err := NewWorker(repository, sender, tokens, WorkerConfig{
		PollInterval: time.Second, RetryBaseDelay: time.Second, ProcessingLease: time.Minute,
		VerificationTTL: time.Hour, FrontendURL: "https://app.zeexco.test/activate",
	})
	if err != nil {
		t.Fatal(err)
	}
	err = worker.deliver(context.Background(), DeliveryJob{
		ID: "job-1", VerificationID: "00000000-0000-4000-8000-000000000001",
		Email: "ada@example.com", FirstName: "Ada", ExpiresAt: now.Add(time.Hour),
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if sender.to != "ada@example.com" || sender.idempotencyKey != "job-1" ||
		!strings.Contains(sender.html, "https://app.zeexco.test/activate?token=") ||
		repository.sentID != "email_123" {
		t.Fatalf("sender=%#v repository=%#v", sender, repository)
	}
}
