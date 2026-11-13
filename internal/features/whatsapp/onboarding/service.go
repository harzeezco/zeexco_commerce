package onboarding

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/mail"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/harzeezco/zeexco_commerce/internal/features/authentication/models"
	"github.com/harzeezco/zeexco_commerce/internal/features/identity"
	whatsappmodels "github.com/harzeezco/zeexco_commerce/internal/features/whatsapp/models"
)

const (
	welcomeMessage     = "Welcome to Zeexco 👋\n\nI can help you organize your products, customer conversations and sales.\n\nLet's get your business set up."
	completionMessage  = "You're all set 🎉\n\nSell faster and sell more by creating your first product."
	productPlaceholder = "Product setup is coming next."
	deliveryClaimLease = time.Minute
)

type SessionRepository interface {
	PrepareWelcome(context.Context, string, string, []byte, time.Time, time.Time, time.Time) (WelcomePreparation, error)
	MarkWelcomeSent(context.Context, string, time.Time) error
	ReleaseWelcomeClaim(context.Context, string) error
	FindByToken(context.Context, []byte) (Session, error)
	Complete(context.Context, []byte, models.RegistrationInput, time.Time) (Completion, error)
	ClaimCompletionForUser(context.Context, string, time.Time, time.Time) (Session, bool, error)
	MarkCompletionSent(context.Context, string, time.Time) error
	ReleaseCompletionClaim(context.Context, string) error
}

type MessageSender interface {
	SendCTAURLMessage(context.Context, string, string, string, string) error
	SendReplyButtonMessage(context.Context, string, string, string, string) error
	SendText(context.Context, string, string) error
}

type FlowMessageSender interface {
	SendFlowMessage(context.Context, string, string, string, string, string) error
}

type ServiceConfig struct {
	PublicBaseURL              string
	ReturnURL                  string
	TokenTTL                   time.Duration
	MerchantRegistrationFlowID string
	Logger                     *log.Logger
}

type Service struct {
	repository                 SessionRepository
	sender                     MessageSender
	publicBaseURL              string
	returnURL                  string
	tokenTTL                   time.Duration
	merchantRegistrationFlowID string
	now                        func() time.Time
	logger                     *log.Logger
}

type outboundDeliveryError struct{ err error }

func (e *outboundDeliveryError) Error() string { return "WhatsApp outbound delivery failed" }
func (e *outboundDeliveryError) Unwrap() error { return e.err }

func NewService(repository SessionRepository, sender MessageSender, cfg ServiceConfig) (*Service, error) {
	if repository == nil || sender == nil {
		return nil, errors.New("onboarding repository and sender are required")
	}
	baseURL, err := validateHTTPSURL(cfg.PublicBaseURL)
	if err != nil {
		return nil, fmt.Errorf("public onboarding URL: %w", err)
	}
	parsedBaseURL, _ := url.Parse(baseURL)
	if (parsedBaseURL.Path != "" && parsedBaseURL.Path != "/") || parsedBaseURL.RawQuery != "" {
		return nil, errors.New("public onboarding URL must be an HTTPS origin without path or query")
	}
	returnURL, err := validateHTTPSURL(cfg.ReturnURL)
	if err != nil {
		return nil, fmt.Errorf("WhatsApp return URL: %w", err)
	}
	if cfg.TokenTTL <= 0 {
		return nil, errors.New("positive onboarding token lifetime is required")
	}
	flowID := strings.TrimSpace(cfg.MerchantRegistrationFlowID)
	if flowID != "" {
		if _, ok := sender.(FlowMessageSender); !ok {
			return nil, errors.New("configured WhatsApp registration Flow requires Flow message sender support")
		}
		for _, character := range flowID {
			if character < '0' || character > '9' {
				return nil, errors.New("WhatsApp registration Flow ID must be numeric")
			}
		}
	}
	logger := cfg.Logger
	if logger == nil {
		logger = log.Default()
	}
	return &Service{
		repository: repository, sender: sender,
		publicBaseURL: parsedBaseURL.Scheme + "://" + parsedBaseURL.Host, returnURL: returnURL,
		tokenTTL: cfg.TokenTTL, merchantRegistrationFlowID: flowID,
		now: time.Now, logger: logger,
	}, nil
}

func (s *Service) HandleIdentityOutcome(
	ctx context.Context,
	message whatsappmodels.IncomingMessage,
	resolution identity.Resolution,
	processed bool,
) error {
	// Identity resolution is already durable for a duplicate message. Never
	// repeat its outbound side effects; a later distinct inbound message or the
	// onboarding success page can retry a persisted pending delivery.
	if !processed {
		return nil
	}
	if !resolution.Registered {
		return s.handleInboundOutcomeError(
			s.startOrRetryWelcome(ctx, message.SenderWhatsAppID, message.BusinessPhoneNumberID),
		)
	}
	if err := s.deliverCompletion(ctx, resolution.UserID); err != nil {
		if err := s.handleInboundOutcomeError(err); err != nil {
			return err
		}
	}
	if message.InteractiveButton != nil && message.InteractiveButton.ID == CreateFirstProductAction {
		if err := s.sender.SendText(ctx, message.SenderWhatsAppID, productPlaceholder); err != nil {
			return s.handleInboundOutcomeError(&outboundDeliveryError{err: err})
		}
	}
	return nil
}

func (s *Service) handleInboundOutcomeError(err error) error {
	if err == nil {
		return nil
	}
	var deliveryError *outboundDeliveryError
	if errors.As(err, &deliveryError) {
		s.logger.Printf("whatsapp onboarding outbound delivery pending outcome=retryable")
		return nil
	}
	return err
}

func (s *Service) startOrRetryWelcome(ctx context.Context, providerUserID, businessPhoneNumberID string) error {
	rawToken, tokenHash, err := NewToken()
	if err != nil {
		return fmt.Errorf("generate onboarding token: %w", err)
	}
	now := s.now().UTC()
	preparation, err := s.repository.PrepareWelcome(
		ctx, providerUserID, businessPhoneNumberID, tokenHash,
		now, now.Add(s.tokenTTL), now.Add(-deliveryClaimLease),
	)
	if err != nil {
		return err
	}
	if !preparation.Send {
		return nil
	}
	if s.merchantRegistrationFlowID != "" {
		flowSender := s.sender.(FlowMessageSender)
		if err := flowSender.SendFlowMessage(
			ctx, providerUserID, welcomeMessage, "Get started",
			s.merchantRegistrationFlowID, rawToken,
		); err == nil {
			if err := s.repository.MarkWelcomeSent(ctx, preparation.Session.ID, s.now().UTC()); err != nil {
				return err
			}
			return nil
		}
		s.logger.Printf("whatsapp onboarding operation=send_registration_flow outcome=fallback")
	}
	onboardingURL := s.publicBaseURL + "/register?token=" + url.QueryEscape(rawToken)
	if err := s.sender.SendCTAURLMessage(ctx, providerUserID, welcomeMessage, "Get started", onboardingURL); err != nil {
		_ = s.repository.ReleaseWelcomeClaim(context.Background(), preparation.Session.ID)
		return &outboundDeliveryError{err: err}
	}
	if err := s.repository.MarkWelcomeSent(ctx, preparation.Session.ID, s.now().UTC()); err != nil {
		return err
	}
	return nil
}

func (s *Service) Validate(ctx context.Context, rawToken string) (Session, error) {
	if ValidateToken(rawToken) != nil {
		return Session{}, ErrMalformedToken
	}
	session, err := s.repository.FindByToken(ctx, TokenDigest(rawToken))
	if err != nil {
		return Session{}, err
	}
	if session.Status == StatusFirstProductPending {
		return session, ErrSessionConsumed
	}
	if session.Status == StatusExpired ||
		(session.Status == StatusRegistrationPending && !s.now().UTC().Before(session.ExpiresAt)) {
		return session, ErrSessionExpired
	}
	if session.Status != StatusRegistrationPending {
		return Session{}, ErrInvalidSession
	}
	return session, nil
}

type RegisterCommand struct {
	FirstName    string
	LastName     string
	BusinessName string
	Email        string
}

func (s *Service) Complete(ctx context.Context, rawToken string, command RegisterCommand) (Completion, error) {
	if ValidateToken(rawToken) != nil {
		return Completion{}, ErrMalformedToken
	}
	input, err := prepareRegistration(command)
	if err != nil {
		return Completion{}, err
	}
	completion, err := s.repository.Complete(ctx, TokenDigest(rawToken), input, s.now().UTC())
	if err != nil {
		return Completion{}, err
	}
	if err := s.deliverCompletion(ctx, completion.Registration.User.ID); err != nil {
		s.logger.Printf("whatsapp onboarding completion delivery pending outcome=retryable")
	}

	return completion, nil
}

func (s *Service) Register(ctx context.Context, rawToken string, command RegisterCommand) (*models.Registration, error) {
	completion, err := s.Complete(ctx, rawToken, command)
	if err != nil {
		return nil, err
	}
	return &completion.Registration, nil
}

func prepareRegistration(command RegisterCommand) (models.RegistrationInput, error) {
	command.FirstName = strings.Join(strings.Fields(command.FirstName), " ")
	command.LastName = strings.Join(strings.Fields(command.LastName), " ")
	command.BusinessName = strings.Join(strings.Fields(command.BusinessName), " ")
	command.Email = strings.ToLower(strings.TrimSpace(command.Email))
	if command.FirstName == "" {
		return models.RegistrationInput{}, validationError("first_name", "first name is required")
	}
	if utf8.RuneCountInString(command.FirstName) > 100 {
		return models.RegistrationInput{}, validationError("first_name", "first name must be 100 characters or fewer")
	}
	if command.LastName == "" {
		return models.RegistrationInput{}, validationError("last_name", "last name is required")
	}
	if utf8.RuneCountInString(command.LastName) > 100 {
		return models.RegistrationInput{}, validationError("last_name", "last name must be 100 characters or fewer")
	}
	if command.BusinessName == "" {
		return models.RegistrationInput{}, validationError("business_name", "business name is required")
	}
	if utf8.RuneCountInString(command.BusinessName) > 200 {
		return models.RegistrationInput{}, validationError("business_name", "business name must be 200 characters or fewer")
	}
	if command.Email != "" {
		parsed, err := mail.ParseAddress(command.Email)
		if len(command.Email) > 255 || err != nil || parsed.Address != command.Email {
			return models.RegistrationInput{}, validationError("email", "email address is invalid")
		}
	}
	return models.RegistrationInput{
		FirstName: command.FirstName, LastName: command.LastName,
		BusinessName: command.BusinessName, Email: command.Email,
	}, nil
}

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

func validationError(field, message string) error {
	return &ValidationError{Field: field, Message: message}
}

func (s *Service) RetryCompletion(ctx context.Context, userID string) {
	if err := s.deliverCompletion(ctx, userID); err != nil {
		s.logger.Printf("whatsapp onboarding completion delivery pending outcome=retryable")
	}
}

func (s *Service) deliverCompletion(ctx context.Context, userID string) error {
	now := s.now().UTC()
	session, claimed, err := s.repository.ClaimCompletionForUser(ctx, userID, now, now.Add(-deliveryClaimLease))
	if err != nil || !claimed {
		return err
	}
	if err := s.sender.SendReplyButtonMessage(
		ctx, session.ProviderUserID, completionMessage,
		CreateFirstProductAction, "Create first product",
	); err != nil {
		_ = s.repository.ReleaseCompletionClaim(context.Background(), session.ID)
		return &outboundDeliveryError{err: err}
	}
	return s.repository.MarkCompletionSent(ctx, session.ID, s.now().UTC())
}

func (s *Service) ReturnURL() string { return s.returnURL }

func validateHTTPSURL(value string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return "", errors.New("must be an absolute HTTPS URL without credentials or fragment")
	}
	return parsed.String(), nil
}
