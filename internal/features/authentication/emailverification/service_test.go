package emailverification

import (
	"context"
	"errors"
	"testing"
	"time"
)

type verificationRepositoryFunc func(context.Context, string, time.Time) error

func (f verificationRepositoryFunc) Verify(ctx context.Context, id string, now time.Time) error {
	return f(ctx, id, now)
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
	service, err := NewService(verificationRepositoryFunc(func(context.Context, string, time.Time) error {
		committed = true
		return nil
	}), tokens)
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
	service, err := NewService(verificationRepositoryFunc(func(context.Context, string, time.Time) error {
		return ErrVerificationConsumed
	}), tokens)
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
