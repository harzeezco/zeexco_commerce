package config

import "testing"

func TestLoadReadsWhatsAppOutboundConfiguration(t *testing.T) {
	t.Setenv("WHATSAPP_ACCESS_TOKEN", "test-access-token")
	t.Setenv("WHATSAPP_PHONE_NUMBER_ID", "test-phone-number-id")
	t.Setenv("META_GRAPH_API_VERSION", "v99.0")

	cfg := Load()

	if cfg.WhatsAppAccessToken != "test-access-token" {
		t.Fatalf("WhatsAppAccessToken = %q, want configured value", cfg.WhatsAppAccessToken)
	}
	if cfg.WhatsAppPhoneNumberID != "test-phone-number-id" {
		t.Fatalf("WhatsAppPhoneNumberID = %q, want configured value", cfg.WhatsAppPhoneNumberID)
	}
	if cfg.MetaGraphAPIVersion != "v99.0" {
		t.Fatalf("MetaGraphAPIVersion = %q, want configured value", cfg.MetaGraphAPIVersion)
	}
}

func TestLoadUsesSafeAuthenticationDefaults(t *testing.T) {
	t.Setenv("AUTH_ACCESS_TOKEN_TTL", "")
	t.Setenv("AUTH_REFRESH_TOKEN_TTL", "")
	t.Setenv("AUTH_REFRESH_COOKIE_SECURE", "")
	t.Setenv("AUTH_REFRESH_COOKIE_SAME_SITE", "")
	cfg := Load()
	if cfg.AuthAccessTokenTTL != "15m" || cfg.AuthRefreshTokenTTL != "720h" ||
		cfg.AuthRefreshCookieSecure != "true" || cfg.AuthRefreshCookieSameSite != "Lax" {
		t.Fatalf("authentication defaults = %#v", cfg)
	}
}

func TestLoadUsesPublicFrontendOriginWithLegacyFallback(t *testing.T) {
	t.Setenv("PUBLIC_FRONTEND_BASE_URL", "https://www.zeexco.com")
	t.Setenv("WHATSAPP_ONBOARDING_PUBLIC_BASE_URL", "https://legacy.example")
	if got := Load().PublicFrontendBaseURL; got != "https://www.zeexco.com" {
		t.Fatalf("PublicFrontendBaseURL=%q", got)
	}
	t.Setenv("PUBLIC_FRONTEND_BASE_URL", "")
	if got := Load().PublicFrontendBaseURL; got != "https://legacy.example" {
		t.Fatalf("legacy fallback=%q", got)
	}
}

func TestLoadReadsWhatsAppFlowKeyConfiguration(t *testing.T) {
	t.Setenv("WHATSAPP_FLOW_PRIVATE_KEY", "private-pem-marker")
	t.Setenv("WHATSAPP_FLOW_PRIVATE_KEY_PASSPHRASE", "passphrase-marker")
	cfg := Load()
	if cfg.WhatsAppFlowPrivateKey != "private-pem-marker" || cfg.WhatsAppFlowKeyPassphrase != "passphrase-marker" {
		t.Fatalf("Flow key configuration was not loaded")
	}
}

func TestLoadReadsWhatsAppMerchantRegistrationFlowID(t *testing.T) {
	t.Setenv("WHATSAPP_MERCHANT_REGISTRATION_FLOW_ID", "987654321")
	if got := Load().WhatsAppMerchantRegistrationFlowID; got != "987654321" {
		t.Fatalf("WhatsAppMerchantRegistrationFlowID=%q", got)
	}
}

func TestLoadUsesPublishedWhatsAppFlowModeByDefault(t *testing.T) {
	t.Setenv("WHATSAPP_FLOW_MODE", "")
	if got := Load().WhatsAppFlowMode; got != "published" {
		t.Fatalf("WhatsAppFlowMode=%q, want published", got)
	}
}

func TestLoadReadsWhatsAppFlowMode(t *testing.T) {
	t.Setenv("WHATSAPP_FLOW_MODE", "draft")
	if got := Load().WhatsAppFlowMode; got != "draft" {
		t.Fatalf("WhatsAppFlowMode=%q, want draft", got)
	}
}
