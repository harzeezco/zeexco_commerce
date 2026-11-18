package emailverification

import (
	"strings"
	"testing"
	"time"
)

func TestActivationEmailEscapesInputAndUsesTokenExpiry(t *testing.T) {
	expiry := time.Date(2026, time.November, 15, 14, 30, 0, 0, time.UTC)
	email, err := renderVerificationEmail(`<script>alert("hello")</script>`, "https://example.com/verify?token=abc&source=email", expiry, expiry.Add(-10*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(email.HTMLBody, "<script>") || !strings.Contains(email.HTMLBody, "&lt;script&gt;") {
		t.Fatal("recipient name was not escaped")
	}
	if strings.Count(email.HTMLBody, `href="https://example.com/verify?token=abc&amp;source=email"`) != 2 {
		t.Fatal("button and fallback must use the same activation URL")
	}
	for _, body := range []string{email.HTMLBody, email.TextBody} {
		if !strings.Contains(body, "This link expires in 10 minutes.") || !strings.Contains(body, "safely ignore this email") {
			t.Fatal("activation email is missing its actual expiry or ignore notice")
		}
	}
	if email.Subject != "Activate your Zico account" || strings.Contains(email.HTMLBody, "ZgotmplZ") {
		t.Fatal("invalid activation subject or blocked template URL")
	}
}

func TestActivationEmailRemainingMinutes(t *testing.T) {
	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		remaining time.Duration
		want      string
	}{
		{time.Minute, "1 minute"},
		{time.Second, "1 minute"},
		{10 * time.Minute, "10 minutes"},
		{10*time.Minute + time.Second, "11 minutes"},
		{24 * time.Hour, "1440 minutes"},
	} {
		t.Run(test.remaining.String(), func(t *testing.T) {
			email, err := renderVerificationEmail("Ada", "https://example.com/verify", now.Add(test.remaining), now)
			if err != nil {
				t.Fatal(err)
			}
			for _, body := range []string{email.HTMLBody, email.TextBody} {
				if !strings.Contains(body, "This link expires in "+test.want+".") {
					t.Fatalf("missing remaining lifetime %q", test.want)
				}
			}
		})
	}
	for _, expiry := range []time.Time{{}, now, now.Add(-time.Minute)} {
		if _, err := renderVerificationEmail("Ada", "https://example.com/verify", expiry, now); err == nil {
			t.Fatal("non-future expiry was accepted")
		}
	}
}

func TestWelcomeEmailUsesZicoDesignAndLoginURL(t *testing.T) {
	email, err := renderWelcomeEmail("Ada & Co", "https://example.com/login")
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"WELCOME ABOARD", "A fresh start for your business.", "Hi Ada &amp; Co,", "Make it yours", "Add your products", "Set up my business", `href="https://example.com/login"`, `src="cid:zico-logo"`, "https://zeexco.com/privacy", "mailto:support@zeexco.com"} {
		if !strings.Contains(email.HTMLBody, fragment) {
			t.Errorf("welcome HTML missing %q", fragment)
		}
	}
	if email.Subject != "Welcome to Zico" || strings.Contains(email.HTMLBody, "ACCOUNT ACTIVATION") || strings.Contains(email.HTMLBody, "ZgotmplZ") {
		t.Fatal("welcome email used invalid or activation content")
	}
	if !strings.Contains(email.TextBody, "Your account is active") || !strings.Contains(email.TextBody, "https://example.com/login") {
		t.Fatal("plain-text welcome email is missing account status or login URL")
	}
}
