package emailverification

import (
	"context"
	"errors"
	"testing"
	"time"
)

type verificationRepositoryStub struct {
	verify          func(context.Context, string, time.Time) error
	queued          bool
	requestedUserID string
}

func (r *verificationRepositoryStub) Verify(ctx context.Context, id string, now time.Time) error {
	return r.verify(ctx, id, now)
}

func (r *verificationRepositoryStub) RequestVerification(_ context.Context, userID string, _, _ time.Time) (bool, error) {
	r.requestedUserID = userID
	return r.queued, nil
}

type recordingNotifier struct{ calls int }

func (n *recordingNotifier) Notify() { n.calls++ }

func TestServiceVerificationCommitsBeforeAsynchronousDeliveryNotification(t *testing.T) {
	tokens, err := NewTokenManager([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	expiresAt := time.Now().UTC().Add(time.Hour)
	raw, err := tokens.Mint("00000000-0000-4000-8000-000000000001", expiresAt)
	if err != nil {
		t.Fatal(err)
	}
	committed := false
	service, err := NewService(&verificationRepositoryStub{verify: func(context.Context, string, time.Time) error {
		committed = true
		return nil
	}}, tokens, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	notifier := &recordingNotifier{}
	service.SetNotifier(notifier)
	if err := service.Verify(context.Background(), raw); err != nil {
		t.Fatal(err)
	}
	if !committed || notifier.calls != 1 {
		t.Fatalf("committed=%t notifications=%d", committed, notifier.calls)
	}
}

func TestServiceDoesNotNotifyWorkerWhenVerificationFails(t *testing.T) {
	tokens, err := NewTokenManager([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := tokens.Mint("00000000-0000-4000-8000-000000000001", time.Now().UTC().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewService(&verificationRepositoryStub{verify: func(context.Context, string, time.Time) error {
		return ErrVerificationConsumed
	}}, tokens, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	notifier := &recordingNotifier{}
	service.SetNotifier(notifier)
	if err := service.Verify(context.Background(), raw); !errors.Is(err, ErrVerificationConsumed) {
		t.Fatalf("error=%v", err)
	}
	if notifier.calls != 0 {
		t.Fatalf("notifications=%d", notifier.calls)
	}
}

func TestServiceRejectsExpiredSignedTokenBeforeRepository(t *testing.T) {
	tokens, _ := NewTokenManager([]byte("0123456789abcdef0123456789abcdef"))
	expiresAt := time.Now().UTC().Add(time.Hour)
	raw, err := tokens.Mint("00000000-0000-4000-8000-000000000001", expiresAt)
	if err != nil {
		t.Fatal(err)
	}
	called := false
	repository := &verificationRepositoryStub{verify: func(context.Context, string, time.Time) error {
		called = true
		return nil
	}}
	service, err := NewService(repository, tokens, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return expiresAt }
	if err := service.Verify(context.Background(), raw); !errors.Is(err, ErrVerificationExpired) || called {
		t.Fatalf("error=%v repository called=%t", err, called)
	}
}

func TestServiceRequestsPendingVerificationAndNotifiesWorker(t *testing.T) {
	tokens, _ := NewTokenManager([]byte("0123456789abcdef0123456789abcdef"))
	repository := &verificationRepositoryStub{
		verify: func(context.Context, string, time.Time) error { return nil },
		queued: true,
	}
	service, err := NewService(repository, tokens, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	notifier := &recordingNotifier{}
	service.SetNotifier(notifier)
	if err := service.Request(context.Background(), "00000000-0000-4000-8000-000000000001"); err != nil {
		t.Fatal(err)
	}
	if repository.requestedUserID == "" || notifier.calls != 1 {
		t.Fatalf("requested user=%q notifications=%d", repository.requestedUserID, notifier.calls)
	}
}
