package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/harzeezco/zeexco_commerce/internal/features/identity"
	"github.com/harzeezco/zeexco_commerce/internal/features/whatsapp/ingestion"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestWhatsAppIdentityRepositoryResolvesExistingIdentity(t *testing.T) {
	queryer := &fakeQueryRower{
		row: fakeRow{scan: func(destinations ...any) error {
			*destinations[0].(*string) = "user-id-1"
			return nil
		}},
	}
	repository := NewWhatsAppIdentityRepository(queryer)

	resolution, err := repository.Resolve(context.Background(), "provider-user-1")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if !resolution.Registered || resolution.UserID != "user-id-1" {
		t.Fatalf("resolution = %#v", resolution)
	}
	if len(queryer.args) != 2 ||
		queryer.args[0] != metaWhatsAppProvider ||
		queryer.args[1] != "provider-user-1" {
		t.Fatalf("query arguments = %#v", queryer.args)
	}
}

func TestWhatsAppIdentityRepositoryReturnsUnknownWithoutWriting(t *testing.T) {
	queryer := &fakeQueryRower{row: fakeRow{scan: func(...any) error {
		return pgx.ErrNoRows
	}}}
	repository := NewWhatsAppIdentityRepository(queryer)

	resolution, err := repository.Resolve(context.Background(), "unknown-provider-user")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolution.Registered || resolution.UserID != "" {
		t.Fatalf("resolution = %#v", resolution)
	}
	if queryer.queryCalls != 1 {
		t.Fatalf("query calls = %d, want 1", queryer.queryCalls)
	}
}

func TestWhatsAppMessageRepositoryStoresNewAndDetectsDuplicate(t *testing.T) {
	newQueryer := &fakeQueryRower{
		row: fakeRow{scan: func(destinations ...any) error {
			*destinations[0].(*string) = "stored-row-id"
			return nil
		}},
	}
	repository := newWhatsAppMessageRepository(newQueryer, nil)
	message := repositoryTestMessage()
	message.SourceWebhookEventID = "source-event-id"

	stored, err := repository.Store(context.Background(), message)
	if err != nil {
		t.Fatalf("Store() error = %v", err)
	}
	if !stored {
		t.Fatal("Store() stored = false, want true")
	}
	if !strings.Contains(newQueryer.sql, "ON CONFLICT (provider_message_id) DO NOTHING") {
		t.Fatalf("insert SQL does not use atomic conflict handling: %s", newQueryer.sql)
	}
	if len(newQueryer.args) != 9 {
		t.Fatalf("insert arguments = %d, want 9", len(newQueryer.args))
	}
	if newQueryer.args[0] != message.ProviderMessageID ||
		newQueryer.args[1] != message.SenderProviderID ||
		newQueryer.args[2] != message.BusinessPhoneNumberID {
		t.Fatalf("insert identity arguments = %#v", newQueryer.args[:3])
	}

	var content ingestion.Content
	encodedContent, ok := newQueryer.args[4].([]byte)
	if !ok {
		t.Fatalf("normalized content type = %T, want []byte", newQueryer.args[4])
	}
	if err := json.Unmarshal(encodedContent, &content); err != nil {
		t.Fatalf("decode normalized content: %v", err)
	}
	if content.Text != message.Content.Text {
		t.Fatalf("normalized content = %#v", content)
	}
	if newQueryer.args[8] != message.SourceWebhookEventID {
		t.Fatalf("source event argument = %#v", newQueryer.args[8])
	}

	duplicateQueryer := &fakeQueryRower{row: fakeRow{scan: func(...any) error {
		return pgx.ErrNoRows
	}}}
	duplicateRepository := newWhatsAppMessageRepository(duplicateQueryer, nil)
	stored, err = duplicateRepository.Store(context.Background(), message)
	if err != nil {
		t.Fatalf("duplicate Store() error = %v", err)
	}
	if stored {
		t.Fatal("duplicate Store() stored = true, want false")
	}
}

func TestRepositoriesReturnUnexpectedDatabaseErrors(t *testing.T) {
	databaseError := errors.New("simulated database failure")
	queryer := &fakeQueryRower{row: fakeRow{scan: func(...any) error {
		return databaseError
	}}}

	identityRepository := NewWhatsAppIdentityRepository(queryer)
	if _, err := identityRepository.Resolve(context.Background(), "provider-user"); !errors.Is(err, databaseError) {
		t.Fatalf("identity error = %v, want database error", err)
	}

	messageRepository := newWhatsAppMessageRepository(queryer, nil)
	if _, err := messageRepository.Store(context.Background(), repositoryTestMessage()); !errors.Is(err, databaseError) {
		t.Fatalf("message error = %v, want database error", err)
	}
}

func TestMessageRepositoryCompletesReceivedIdentityResolution(t *testing.T) {
	tx := &fakeProcessingTransaction{
		row: fakeRow{scan: func(destinations ...any) error {
			*destinations[0].(*string) = ingestion.ProcessingStatusReceived
			*destinations[1].(**string) = nil
			return nil
		}},
	}
	repository := newWhatsAppMessageRepository(&fakeQueryRower{}, func(context.Context) (processingTransaction, error) {
		return tx, nil
	})
	want := identity.Resolution{Registered: true, UserID: "00000000-0000-4000-8000-000000000001"}
	resolveCalls := 0

	resolution, processed, err := repository.CompleteIdentityResolution(
		context.Background(),
		"message-1",
		func(_ context.Context, scope any) (identity.Resolution, error) {
			resolveCalls++
			if scope != tx {
				t.Fatalf("processing scope = %T, want transaction", scope)
			}
			return want, nil
		},
	)
	if err != nil || !processed || resolution != want {
		t.Fatalf("resolution = %#v, processed = %v, error = %v", resolution, processed, err)
	}
	if resolveCalls != 1 || tx.execCalls != 1 || tx.commitCalls != 1 {
		t.Fatalf("resolve=%d exec=%d commit=%d", resolveCalls, tx.execCalls, tx.commitCalls)
	}
	if tx.execArgs[1] != ingestion.ProcessingStatusIdentityResolved || tx.execArgs[2] != want.UserID {
		t.Fatalf("completion arguments = %#v", tx.execArgs)
	}
}

func TestMessageRepositorySkipsCompletedIdentityResolution(t *testing.T) {
	userID := "00000000-0000-4000-8000-000000000001"
	tx := &fakeProcessingTransaction{
		row: fakeRow{scan: func(destinations ...any) error {
			*destinations[0].(*string) = ingestion.ProcessingStatusIdentityResolved
			*destinations[1].(**string) = &userID
			return nil
		}},
	}
	repository := newWhatsAppMessageRepository(&fakeQueryRower{}, func(context.Context) (processingTransaction, error) {
		return tx, nil
	})
	resolveCalls := 0

	resolution, processed, err := repository.CompleteIdentityResolution(
		context.Background(),
		"message-1",
		func(context.Context, any) (identity.Resolution, error) {
			resolveCalls++
			return identity.Resolution{}, nil
		},
	)
	if err != nil || processed || !resolution.Registered || resolution.UserID != userID {
		t.Fatalf("resolution = %#v, processed = %v, error = %v", resolution, processed, err)
	}
	if resolveCalls != 0 || tx.execCalls != 0 || tx.commitCalls != 1 {
		t.Fatalf("resolve=%d exec=%d commit=%d", resolveCalls, tx.execCalls, tx.commitCalls)
	}
}

func TestMessageRepositoryLeavesReceivedWhenIdentityResolutionFails(t *testing.T) {
	tx := &fakeProcessingTransaction{
		row: fakeRow{scan: func(destinations ...any) error {
			*destinations[0].(*string) = ingestion.ProcessingStatusReceived
			*destinations[1].(**string) = nil
			return nil
		}},
	}
	repository := newWhatsAppMessageRepository(&fakeQueryRower{}, func(context.Context) (processingTransaction, error) {
		return tx, nil
	})
	databaseError := errors.New("temporary identity lookup failure")

	_, processed, err := repository.CompleteIdentityResolution(
		context.Background(),
		"message-1",
		func(context.Context, any) (identity.Resolution, error) {
			return identity.Resolution{}, databaseError
		},
	)
	if !errors.Is(err, databaseError) || processed {
		t.Fatalf("processed = %v, error = %v", processed, err)
	}
	if tx.execCalls != 0 || tx.commitCalls != 0 || tx.rollbackCalls != 1 {
		t.Fatalf("exec=%d commit=%d rollback=%d", tx.execCalls, tx.commitCalls, tx.rollbackCalls)
	}
}

func repositoryTestMessage() ingestion.Message {
	timestamp := time.Unix(1700000000, 0).UTC()
	return ingestion.Message{
		ProviderMessageID:     "provider-message-id-1",
		SenderProviderID:      "sender-provider-id-1",
		BusinessPhoneNumberID: "business-phone-id-1",
		MessageType:           "text",
		Content:               ingestion.Content{Text: "sanitized content"},
		ProcessingStatus:      ingestion.ProcessingStatusReceived,
		ProviderTimestamp:     &timestamp,
		ReceivedAt:            time.Unix(1800000000, 0).UTC(),
	}
}

type fakeQueryRower struct {
	row        pgx.Row
	queryCalls int
	sql        string
	args       []any
}

func (f *fakeQueryRower) QueryRow(
	_ context.Context,
	sql string,
	args ...any,
) pgx.Row {
	f.queryCalls++
	f.sql = sql
	f.args = append([]any(nil), args...)
	return f.row
}

type fakeRow struct {
	scan func(destinations ...any) error
}

type fakeProcessingTransaction struct {
	row           pgx.Row
	execCalls     int
	execArgs      []any
	commitCalls   int
	rollbackCalls int
}

func (t *fakeProcessingTransaction) QueryRow(context.Context, string, ...any) pgx.Row {
	return t.row
}

func (t *fakeProcessingTransaction) Exec(_ context.Context, _ string, arguments ...any) (pgconn.CommandTag, error) {
	t.execCalls++
	t.execArgs = append([]any(nil), arguments...)
	return pgconn.NewCommandTag("UPDATE 1"), nil
}

func (t *fakeProcessingTransaction) Commit(context.Context) error {
	t.commitCalls++
	return nil
}

func (t *fakeProcessingTransaction) Rollback(context.Context) error {
	t.rollbackCalls++
	return nil
}

func (r fakeRow) Scan(destinations ...any) error {
	return r.scan(destinations...)
}
