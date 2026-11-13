package flows

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	authmodels "github.com/harzeezco/zeexco_commerce/internal/features/authentication/models"
	"github.com/harzeezco/zeexco_commerce/internal/features/whatsapp/onboarding"
)

func TestActionHandlerSupportsPingInitAndRejectsUnknownAction(t *testing.T) {
	handler := NewActionHandler(nil)
	ping, err := handler.Handle(context.Background(), []byte(`{"version":"3.0","action":"ping"}`))
	if err != nil || ping.(map[string]any)["data"].(map[string]string)["status"] != "active" {
		t.Fatalf("ping=%#v error=%v", ping, err)
	}
	initResponse, err := handler.Handle(context.Background(), []byte(`{"version":"3.0","action":"INIT"}`))
	if err != nil || initResponse.(map[string]any)["screen"] != RegistrationScreen {
		t.Fatalf("INIT=%#v error=%v", initResponse, err)
	}
	if _, err := handler.Handle(context.Background(), []byte(`{"version":"3.0","action":"unknown"}`)); !errors.Is(err, ErrUnsupportedAction) {
		t.Fatalf("unknown action error=%v", err)
	}
}

func TestActionHandlerRejectsUnknownFieldsAndTrailingJSON(t *testing.T) {
	handler := NewActionHandler(nil)
	for _, payload := range [][]byte{
		[]byte(`{"version":"3.0","action":"ping","unexpected":true}`),
		[]byte(`{"version":"3.0","action":"ping"}{"action":"ping"}`),
	} {
		if _, err := handler.Handle(context.Background(), payload); !errors.Is(err, ErrInvalidPayload) {
			t.Fatalf("payload=%s error=%v", payload, err)
		}
	}
}

func TestDataExchangeUsesOpaqueFlowTokenAsOnboardingCorrelation(t *testing.T) {
	registrar := &fakeRegistrar{}
	handler := NewActionHandler(registrar)
	response, err := handler.Handle(context.Background(), []byte(`{
		"version":"3.0","action":"data_exchange","screen":"REGISTRATION",
		"flow_token":"opaque-server-token",
		"data":{"first_name":"Ada","last_name":"Lovelace","business_name":"Commerce"}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if registrar.calls != 1 || registrar.token != "opaque-server-token" || registrar.command.Email != "" {
		t.Fatalf("registrar=%#v", registrar)
	}
	if response.(map[string]any)["screen"] != SuccessScreen {
		t.Fatalf("response=%#v", response)
	}

	withEmail := []byte(`{
		"version":"3.0","action":"data_exchange","screen":"REGISTRATION",
		"flow_token":"opaque-server-token",
		"data":{"first_name":"Ada","last_name":"Lovelace","business_name":"Commerce","email":"ADA@EXAMPLE.COM"}
	}`)
	if _, err := handler.Handle(context.Background(), withEmail); err != nil || registrar.command.Email != "ADA@EXAMPLE.COM" {
		t.Fatalf("optional email command=%#v error=%v", registrar.command, err)
	}
}

func TestDataExchangeMapsDomainConflictsWithoutLeakingInternals(t *testing.T) {
	tests := []struct {
		err  error
		code string
	}{
		{authmodels.ErrEmailAlreadyRegistered, "EMAIL_ALREADY_REGISTERED"},
		{onboarding.ErrIdentityConflict, "WHATSAPP_IDENTITY_CONFLICT"},
		{onboarding.ErrSessionConsumed, "ONBOARDING_CONSUMED"},
		{onboarding.ErrSessionExpired, "ONBOARDING_EXPIRED"},
	}
	for _, test := range tests {
		registrar := &fakeRegistrar{err: test.err}
		_, err := NewActionHandler(registrar).Handle(context.Background(), []byte(`{
			"version":"3.0","action":"data_exchange","screen":"REGISTRATION",
			"flow_token":"opaque-server-token",
			"data":{"first_name":"Ada","last_name":"Lovelace","business_name":"Commerce"}
		}`))
		var domain *DomainError
		if !errors.As(err, &domain) || domain.Code != test.code {
			t.Fatalf("error=%v domain=%#v", err, domain)
		}
	}
}

func TestDataExchangeRejectsClientSelectedIdentityAndPassword(t *testing.T) {
	for _, field := range []string{"wa_id", "whatsapp_number", "password"} {
		data, _ := json.Marshal(map[string]string{
			"first_name": "Ada", "last_name": "Lovelace", "business_name": "Commerce", field: "attacker-value",
		})
		payload, _ := json.Marshal(Payload{
			Version: DataAPIVersion, Action: "data_exchange", Screen: RegistrationScreen,
			FlowToken: "opaque-server-token", Data: data,
		})
		registrar := &fakeRegistrar{}
		if _, err := NewActionHandler(registrar).Handle(context.Background(), payload); !errors.Is(err, ErrInvalidPayload) || registrar.calls != 0 {
			t.Fatalf("field=%s error=%v calls=%d", field, err, registrar.calls)
		}
	}
}

type fakeRegistrar struct {
	calls   int
	token   string
	command onboarding.RegisterCommand
	err     error
}

func (f *fakeRegistrar) Complete(_ context.Context, token string, command onboarding.RegisterCommand) (onboarding.Completion, error) {
	f.calls++
	f.token = token
	f.command = command
	return onboarding.Completion{}, f.err
}
