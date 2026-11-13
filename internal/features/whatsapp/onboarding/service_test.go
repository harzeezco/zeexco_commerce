package onboarding

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"strings"
	"testing"
	"time"

	authmodels "github.com/harzeezco/zeexco_commerce/internal/features/authentication/models"
	"github.com/harzeezco/zeexco_commerce/internal/features/identity"
	whatsappmodels "github.com/harzeezco/zeexco_commerce/internal/features/whatsapp/models"
)

func TestUnknownSenderStartsOneOnboardingAndSendsHTTPSCTA(t *testing.T) {
	repository := &fakeSessionRepository{sendWelcome: true}
	sender := &fakeSender{}
	service := newTestService(t, repository, sender)
	message := whatsappmodels.IncomingMessage{SenderWhatsAppID: "verified-wa-id", BusinessPhoneNumberID: "phone-id"}

	if err := service.HandleIdentityOutcome(context.Background(), message, identity.Resolution{}, true); err != nil {
		t.Fatal(err)
	}
	if repository.prepareCalls != 1 || sender.ctaCalls != 1 {
		t.Fatalf("prepare calls=%d CTA calls=%d", repository.prepareCalls, sender.ctaCalls)
	}
	if !strings.HasPrefix(sender.ctaURL, "https://commerce.example/register?token=") || strings.Contains(sender.ctaURL, "verified-wa-id") {
		t.Fatalf("unsafe onboarding URL %q", sender.ctaURL)
	}
	if bytesEqualString(repository.lastDigest, sender.ctaURL) {
		t.Fatal("raw URL token was persisted")
	}

	repository.sendWelcome = false
	if err := service.HandleIdentityOutcome(context.Background(), message, identity.Resolution{}, true); err != nil {
		t.Fatal(err)
	}
	if sender.ctaCalls != 1 {
		t.Fatalf("repeated greeting sent %d welcome messages", sender.ctaCalls)
	}
}

func TestUnknownSenderLaunchesRegistrationFlowWithOpaqueToken(t *testing.T) {
	repository := &fakeSessionRepository{sendWelcome: true}
	sender := &fakeSender{}
	var logs bytes.Buffer
	service, err := NewService(repository, sender, ServiceConfig{
		PublicBaseURL: "https://commerce.example", ReturnURL: "https://wa.me/",
		TokenTTL: time.Hour, MerchantRegistrationFlowID: "987654321",
		Logger: log.New(&logs, "", 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	message := whatsappmodels.IncomingMessage{SenderWhatsAppID: "verified-wa-id", BusinessPhoneNumberID: "phone-id"}

	if err := service.HandleIdentityOutcome(context.Background(), message, identity.Resolution{}, true); err != nil {
		t.Fatal(err)
	}
	if sender.flowCalls != 1 || sender.ctaCalls != 0 || sender.flowID != "987654321" || ValidateToken(sender.flowToken) != nil {
		t.Fatalf("flow calls=%d CTA calls=%d flowID=%q token_valid=%v", sender.flowCalls, sender.ctaCalls, sender.flowID, ValidateToken(sender.flowToken))
	}
	if bytesEqualString(repository.lastDigest, sender.flowToken) || strings.Contains(logs.String(), sender.flowToken) {
		t.Fatal("raw Flow token was persisted or logged")
	}
	if err := service.HandleIdentityOutcome(context.Background(), message, identity.Resolution{}, false); err != nil {
		t.Fatal(err)
	}
	if sender.flowCalls != 1 || repository.prepareCalls != 1 {
		t.Fatalf("duplicate webhook launched Flow again: flow calls=%d prepare calls=%d", sender.flowCalls, repository.prepareCalls)
	}
}

func TestFlowLaunchFailureFallsBackToBrowserCTAWithSameToken(t *testing.T) {
	repository := &fakeSessionRepository{sendWelcome: true}
	sender := &fakeSender{flowError: errors.New("provider rejected Flow")}
	service, err := NewService(repository, sender, ServiceConfig{
		PublicBaseURL: "https://commerce.example", ReturnURL: "https://wa.me/",
		TokenTTL: time.Hour, MerchantRegistrationFlowID: "987654321", Logger: log.New(io.Discard, "", 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	message := whatsappmodels.IncomingMessage{SenderWhatsAppID: "verified-wa-id", BusinessPhoneNumberID: "phone-id"}

	if err := service.HandleIdentityOutcome(context.Background(), message, identity.Resolution{}, true); err != nil {
		t.Fatal(err)
	}
	if sender.flowCalls != 1 || sender.ctaCalls != 1 || !strings.Contains(sender.ctaURL, "token="+sender.flowToken) {
		t.Fatalf("flow calls=%d CTA calls=%d URL=%q", sender.flowCalls, sender.ctaCalls, sender.ctaURL)
	}
}

func TestDuplicateUnknownInboundDoesNotRepeatCTA(t *testing.T) {
	repository := &fakeSessionRepository{sendWelcome: true}
	sender := &fakeSender{}
	service := newTestService(t, repository, sender)
	message := whatsappmodels.IncomingMessage{SenderWhatsAppID: "verified-wa-id", BusinessPhoneNumberID: "phone-id"}

	if err := service.HandleIdentityOutcome(context.Background(), message, identity.Resolution{}, false); err != nil {
		t.Fatal(err)
	}
	if repository.prepareCalls != 0 || sender.ctaCalls != 0 {
		t.Fatalf("duplicate caused prepare calls=%d CTA calls=%d", repository.prepareCalls, sender.ctaCalls)
	}
}

func TestOutboundProviderFailureDoesNotFailDurableInboundProcessing(t *testing.T) {
	repository := &fakeSessionRepository{sendWelcome: true}
	sender := &fakeSender{ctaError: errors.New("provider rejected request")}
	service := newTestService(t, repository, sender)
	message := whatsappmodels.IncomingMessage{SenderWhatsAppID: "verified-wa-id", BusinessPhoneNumberID: "phone-id"}

	if err := service.HandleIdentityOutcome(context.Background(), message, identity.Resolution{}, true); err != nil {
		t.Fatalf("outbound failure propagated into inbound ingestion: %v", err)
	}
	if repository.releaseWelcomeCalls != 1 || sender.ctaCalls != 1 {
		t.Fatalf("release calls=%d CTA calls=%d", repository.releaseWelcomeCalls, sender.ctaCalls)
	}
}

func TestKnownSenderDoesNotStartRegistrationAndDuplicateButtonIsIdempotent(t *testing.T) {
	repository := &fakeSessionRepository{}
	sender := &fakeSender{}
	service := newTestService(t, repository, sender)
	message := whatsappmodels.IncomingMessage{SenderWhatsAppID: "wa", InteractiveButton: &whatsappmodels.IncomingReply{ID: CreateFirstProductAction}}
	resolution := identity.Resolution{Registered: true, UserID: "user-id"}
	if err := service.HandleIdentityOutcome(context.Background(), message, resolution, false); err != nil {
		t.Fatal(err)
	}
	if repository.prepareCalls != 0 || sender.textCalls != 0 {
		t.Fatal("known or duplicate message incorrectly restarted onboarding")
	}
	if err := service.HandleIdentityOutcome(context.Background(), message, resolution, true); err != nil {
		t.Fatal(err)
	}
	if sender.textCalls != 1 || sender.lastText != productPlaceholder {
		t.Fatal("first-product boundary did not return the deterministic placeholder")
	}
}

func TestSessionValidationRejectsInvalidExpiredAndConsumed(t *testing.T) {
	raw, _, _ := NewToken()
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	tests := []struct {
		name    string
		raw     string
		session Session
		want    error
	}{
		{"malformed", "bad", Session{}, ErrMalformedToken},
		{"unknown", raw, Session{}, ErrInvalidSession},
		{"expired", raw, Session{Status: StatusRegistrationPending, ExpiresAt: now}, ErrSessionExpired},
		{"consumed", raw, Session{Status: StatusFirstProductPending, ExpiresAt: now.Add(time.Hour)}, ErrSessionConsumed},
		{"expired state", raw, Session{Status: StatusExpired, ExpiresAt: now.Add(time.Hour)}, ErrSessionExpired},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &fakeSessionRepository{found: test.session}
			if test.name == "unknown" {
				repository.findError = ErrInvalidSession
			}
			service := newTestService(t, repository, &fakeSender{})
			service.now = func() time.Time { return now }
			_, err := service.Validate(context.Background(), test.raw)
			if !errors.Is(err, test.want) {
				t.Fatalf("error=%v want=%v", err, test.want)
			}
		})
	}
}

func TestSuccessfulCompletionInvitesFirstProductAndDeliveryCanRetry(t *testing.T) {
	raw, _, _ := NewToken()
	repository := &fakeSessionRepository{
		completion: Completion{Registration: authmodels.Registration{User: authmodels.User{ID: "user-id"}}},
		claim:      Session{ID: "session-id", ProviderUserID: "wa-id"}, claimCompletion: true,
	}
	sender := &fakeSender{replyError: errors.New("temporary")}
	service := newTestService(t, repository, sender)
	_, err := service.Complete(context.Background(), raw, validWhatsAppRegisterCommand())
	if err != nil {
		t.Fatalf("committed completion must remain successful on outbound failure: %v", err)
	}
	if repository.completeCalls != 1 || repository.releaseCompletionCalls != 1 {
		t.Fatal("completion or retry state was not persisted")
	}
	sender.replyError = nil
	service.RetryCompletion(context.Background(), "user-id")
	if sender.replyCalls != 2 || repository.markCompletionCalls != 1 || sender.replyID != CreateFirstProductAction {
		t.Fatal("completion invitation was not retried and marked delivered")
	}
}

func TestOnboardingTokenIsNeverLogged(t *testing.T) {
	raw, _, _ := NewToken()
	var logs bytes.Buffer
	repository := &fakeSessionRepository{
		completion: Completion{Registration: authmodels.Registration{User: authmodels.User{ID: "user-id"}}},
		claim:      Session{ID: "session-id", ProviderUserID: "wa-id"}, claimCompletion: true,
	}
	sender := &fakeSender{replyError: errors.New("provider failure")}
	service, err := NewService(repository, sender, ServiceConfig{
		PublicBaseURL: "https://commerce.example", ReturnURL: "https://wa.me/", TokenTTL: time.Hour,
		Logger: log.New(&logs, "", 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Register(context.Background(), raw, validWhatsAppRegisterCommand()); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(logs.String(), raw) {
		t.Fatalf("logs exposed onboarding token: %q", logs.String())
	}
}

func TestRegistrationRevalidatesTokenBeforePersistence(t *testing.T) {
	repository := &fakeSessionRepository{}
	service := newTestService(t, repository, &fakeSender{})
	if _, err := service.Register(context.Background(), "malformed", RegisterCommand{}); !errors.Is(err, ErrMalformedToken) {
		t.Fatalf("Register() error=%v", err)
	}
	if repository.completeCalls != 0 {
		t.Fatal("malformed token reached registration persistence")
	}
}

func TestWhatsAppRegistrationValidationAndPasswordlessInput(t *testing.T) {
	tests := []struct {
		name    string
		command RegisterCommand
		field   string
	}{
		{name: "first name", command: RegisterCommand{LastName: "L", BusinessName: "B"}, field: "first_name"},
		{name: "last name", command: RegisterCommand{FirstName: "A", BusinessName: "B"}, field: "last_name"},
		{name: "business", command: RegisterCommand{FirstName: "A", LastName: "L"}, field: "business_name"},
		{name: "malformed optional email", command: RegisterCommand{FirstName: "A", LastName: "L", BusinessName: "B", Email: "not-an-email"}, field: "email"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := prepareRegistration(test.command)
			var validation *ValidationError
			if !errors.As(err, &validation) || validation.Field != test.field {
				t.Fatalf("error=%v, want validation for %s", err, test.field)
			}
		})
	}

	for _, email := range []string{"", " OWNER@Example.com "} {
		input, err := prepareRegistration(RegisterCommand{
			FirstName: " Ada ", LastName: " Lovelace ",
			BusinessName: " Analytical Commerce ", Email: email,
		})
		if err != nil {
			t.Fatalf("email %q: %v", email, err)
		}
		if input.PasswordHash != "" {
			t.Fatal("WhatsApp registration created a password hash")
		}
		if email != "" && input.Email != "owner@example.com" {
			t.Fatalf("normalized email=%q", input.Email)
		}
	}
}

func newTestService(t *testing.T, repository *fakeSessionRepository, sender *fakeSender) *Service {
	t.Helper()
	service, err := NewService(repository, sender, ServiceConfig{
		PublicBaseURL: "https://commerce.example", ReturnURL: "https://wa.me/", TokenTTL: 30 * time.Minute,
		Logger: log.New(io.Discard, "", 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func validWhatsAppRegisterCommand() RegisterCommand {
	return RegisterCommand{
		FirstName: "Ada", LastName: "Lovelace",
		BusinessName: "Analytical Commerce", Email: "a@example.com",
	}
}

type fakeSessionRepository struct {
	prepareCalls, completeCalls, releaseWelcomeCalls, releaseCompletionCalls, markCompletionCalls int
	sendWelcome, claimCompletion                                                                  bool
	lastDigest                                                                                    []byte
	found                                                                                         Session
	completion                                                                                    Completion
	claim                                                                                         Session
	findError                                                                                     error
}

func (f *fakeSessionRepository) PrepareWelcome(_ context.Context, _, _ string, digest []byte, _, _, _ time.Time) (WelcomePreparation, error) {
	f.prepareCalls++
	f.lastDigest = append([]byte(nil), digest...)
	return WelcomePreparation{Session: Session{ID: "session-id"}, Send: f.sendWelcome}, nil
}
func (*fakeSessionRepository) MarkWelcomeSent(context.Context, string, time.Time) error { return nil }
func (f *fakeSessionRepository) ReleaseWelcomeClaim(context.Context, string) error {
	f.releaseWelcomeCalls++
	return nil
}
func (f *fakeSessionRepository) FindByToken(context.Context, []byte) (Session, error) {
	return f.found, f.findError
}
func (f *fakeSessionRepository) Complete(context.Context, []byte, authmodels.RegistrationInput, time.Time) (Completion, error) {
	f.completeCalls++
	return f.completion, nil
}
func (f *fakeSessionRepository) ClaimCompletionForUser(context.Context, string, time.Time, time.Time) (Session, bool, error) {
	return f.claim, f.claimCompletion, nil
}
func (f *fakeSessionRepository) MarkCompletionSent(context.Context, string, time.Time) error {
	f.markCompletionCalls++
	return nil
}
func (f *fakeSessionRepository) ReleaseCompletionClaim(context.Context, string) error {
	f.releaseCompletionCalls++
	return nil
}

type fakeSender struct {
	ctaCalls, flowCalls, replyCalls, textCalls   int
	ctaURL, flowID, flowToken, replyID, lastText string
	ctaError, flowError, replyError              error
}

func (f *fakeSender) SendCTAURLMessage(_ context.Context, _, _, _, target string) error {
	f.ctaCalls++
	f.ctaURL = target
	return f.ctaError
}
func (f *fakeSender) SendReplyButtonMessage(_ context.Context, _, _, id, _ string) error {
	f.replyCalls++
	f.replyID = id
	return f.replyError
}
func (f *fakeSender) SendFlowMessage(_ context.Context, _, _, _, flowID, flowToken string) error {
	f.flowCalls++
	f.flowID = flowID
	f.flowToken = flowToken
	return f.flowError
}
func (f *fakeSender) SendText(_ context.Context, _, body string) error {
	f.textCalls++
	f.lastText = body
	return nil
}

func bytesEqualString(digest []byte, value string) bool {
	return strings.Contains(value, string(digest))
}
