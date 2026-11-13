package app

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/harzeezco/zeexco_commerce/internal/config"
)

func TestFrontendCORSAllowsOnlyConfiguredOrigin(t *testing.T) {
	nextCalls := 0
	handler, err := newCORSHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		nextCalls++
		w.WriteHeader(http.StatusOK)
	}), "https://www.zeexco.com")
	if err != nil {
		t.Fatal(err)
	}

	allowed := httptest.NewRequest(http.MethodOptions, "/api/v1/auth/register", nil)
	allowed.Header.Set("Origin", "https://www.zeexco.com")
	allowedResponse := httptest.NewRecorder()
	handler.ServeHTTP(allowedResponse, allowed)
	if allowedResponse.Code != http.StatusNoContent ||
		allowedResponse.Header().Get("Access-Control-Allow-Origin") != "https://www.zeexco.com" ||
		allowedResponse.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatalf("allowed response status=%d headers=%v", allowedResponse.Code, allowedResponse.Header())
	}

	denied := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", nil)
	denied.Header.Set("Origin", "https://attacker.example")
	deniedResponse := httptest.NewRecorder()
	handler.ServeHTTP(deniedResponse, denied)
	if deniedResponse.Code != http.StatusForbidden || nextCalls != 0 || deniedResponse.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("denied status=%d calls=%d headers=%v", deniedResponse.Code, nextCalls, deniedResponse.Header())
	}
}

func TestFrontendCORSRejectsNonOriginConfiguration(t *testing.T) {
	if _, err := newCORSHandler(http.NotFoundHandler(), "https://www.zeexco.com/register"); err == nil {
		t.Fatal("CORS accepted a URL with a path")
	}
}

func TestLoadAuthenticationRuntimeUsesSafeDefaultsAndMatchingKeys(t *testing.T) {
	privateValue, publicValue := testAuthenticationKeys(t)
	runtime, err := loadAuthenticationRuntime(config.Config{
		AuthJWTPrivateKeyBase64: privateValue,
		AuthJWTPublicKeyBase64:  publicValue,
		AuthJWTIssuer:           "zeexco", AuthJWTAudience: "zeexco-api",
		AuthAccessTokenTTL: "15m", AuthRefreshTokenTTL: "720h",
		AuthRefreshCookieName: "zeexco_refresh", AuthRefreshCookieSecure: "true",
		AuthRefreshCookieSameSite: "Lax",
	})
	if err != nil {
		t.Fatalf("loadAuthenticationRuntime() error = %v", err)
	}
	if runtime.tokens.Lifetime() != 15*time.Minute || runtime.refreshTTL != 30*24*time.Hour ||
		!runtime.cookie.Secure || runtime.cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("runtime = %#v", runtime)
	}
}

func TestLoadAuthenticationRuntimeFailsClosed(t *testing.T) {
	if _, err := loadAuthenticationRuntime(config.Config{}); err == nil {
		t.Fatal("loadAuthenticationRuntime() accepted missing signing keys")
	}
	privateValue, publicValue := testAuthenticationKeys(t)
	_, err := loadAuthenticationRuntime(config.Config{
		AuthJWTPrivateKeyBase64: privateValue, AuthJWTPublicKeyBase64: publicValue,
		AuthJWTIssuer: "zeexco", AuthJWTAudience: "zeexco-api",
		AuthAccessTokenTTL: "15m", AuthRefreshTokenTTL: "720h",
		AuthRefreshCookieName: "zeexco_refresh", AuthRefreshCookieSecure: "false",
		AuthRefreshCookieSameSite: "None",
	})
	if err == nil {
		t.Fatal("loadAuthenticationRuntime() accepted insecure SameSite=None cookie")
	}
}

func TestLoadEmailVerificationRuntimeFailsClosedAndUsesDurations(t *testing.T) {
	secret := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	runtime, err := loadEmailVerificationRuntime(config.Config{
		ResendAPIKey: "re_test", ResendFromEmail: "Zeexco <accounts@example.com>",
		ResendAPIBaseURL:                  "https://api.resend.com",
		EmailVerificationFrontendURL:      "https://www.zeexco.com/activate-account",
		EmailVerificationHMACSecretBase64: secret, EmailVerificationTTL: "24h",
		EmailWorkerPollInterval: "250ms", EmailWorkerRetryBaseDelay: "30s",
		EmailWorkerProcessingLease: "5m",
	})
	if err != nil {
		t.Fatal(err)
	}
	if runtime.ttl != 24*time.Hour || runtime.worker.RetryBaseDelay != 30*time.Second {
		t.Fatalf("runtime=%#v", runtime)
	}
	if _, err := loadEmailVerificationRuntime(config.Config{}); err == nil {
		t.Fatal("missing email verification secrets were accepted")
	}
}

func TestWhatsAppWebhookConfigurationIsOptionalForBrowserRegistration(t *testing.T) {
	enabled, err := whatsAppWebhookConfiguration(config.Config{})
	if err != nil || enabled {
		t.Fatalf("empty WhatsApp configuration enabled=%v error=%v", enabled, err)
	}

	enabled, err = whatsAppWebhookConfiguration(config.Config{
		WhatsAppWebhookVerifyToken: "verify-token",
		MetaAppSecret:              "app-secret",
	})
	if err != nil || !enabled {
		t.Fatalf("complete WhatsApp configuration enabled=%v error=%v", enabled, err)
	}
}

func TestWhatsAppFlowConfigurationIsIndependentAndFailsClosed(t *testing.T) {
	flowOnly := config.Config{
		MetaAppSecret:          "meta-secret",
		WhatsAppFlowPrivateKey: "private-key",
	}
	if enabled, err := whatsAppWebhookConfiguration(flowOnly); err != nil || enabled {
		t.Fatalf("flow-only webhook enabled=%t error=%v", enabled, err)
	}
	if enabled, err := whatsAppFlowConfiguration(flowOnly); err != nil || !enabled {
		t.Fatalf("flow enabled=%t error=%v", enabled, err)
	}
	if _, err := whatsAppFlowConfiguration(config.Config{WhatsAppFlowPrivateKey: "private-key"}); err == nil {
		t.Fatal("Flow private key without META_APP_SECRET must fail")
	}
	if _, err := whatsAppFlowConfiguration(config.Config{WhatsAppFlowKeyPassphrase: "passphrase"}); err == nil {
		t.Fatal("Flow passphrase without private key must fail")
	}
}

func TestNewRejectsPartialWhatsAppConfigurationWithoutVerifyToken(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://unused-during-config-validation")
	t.Setenv("WHATSAPP_WEBHOOK_VERIFY_TOKEN", "")
	t.Setenv("META_APP_SECRET", "test-meta-app-secret")

	application, err := New(context.Background())
	if application != nil {
		application.Close()
		t.Fatal("New() returned an application without a webhook verification token")
	}

	if err == nil {
		t.Fatal("New() error = nil, want missing webhook verification token error")
	}

	if !strings.Contains(err.Error(), "must both be configured") {
		t.Fatalf("New() error = %q, want partial WhatsApp configuration error", err)
	}
}

func TestNewRejectsPartialWhatsAppConfigurationWithoutMetaSecret(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://unused-during-config-validation")
	t.Setenv("WHATSAPP_WEBHOOK_VERIFY_TOKEN", "test-webhook-verification-token")
	t.Setenv("META_APP_SECRET", "")

	application, err := New(context.Background())
	if application != nil {
		application.Close()
		t.Fatal("New() returned an application without a Meta App Secret")
	}

	if err == nil {
		t.Fatal("New() error = nil, want missing Meta App Secret error")
	}

	if !strings.Contains(err.Error(), "must both be configured") {
		t.Fatalf("New() error = %q, want partial WhatsApp configuration error", err)
	}
}

func testAuthenticationKeys(t *testing.T) (string, string) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate keys: %v", err)
	}
	privateDER, _ := x509.MarshalPKCS8PrivateKey(privateKey)
	publicDER, _ := x509.MarshalPKIXPublicKey(publicKey)
	privatePEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER})
	publicPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER})
	return base64.StdEncoding.EncodeToString(privatePEM), base64.StdEncoding.EncodeToString(publicPEM)
}
