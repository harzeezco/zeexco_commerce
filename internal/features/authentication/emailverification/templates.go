package emailverification

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"strings"
)

type RenderedEmail struct {
	Purpose  EmailPurpose
	Subject  string
	HTMLBody string
	TextBody string
}

var verificationEmailTemplate = template.Must(template.New("verification").Parse(`<!doctype html>
<html><body style="margin:0;background:#f5f7fa;font-family:Arial,sans-serif;color:#172033">
<table role="presentation" width="100%" cellspacing="0" cellpadding="0"><tr><td style="padding:32px 16px">
<table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="max-width:600px;margin:auto;background:#ffffff;border-radius:12px">
<tr><td style="padding:36px"><h1 style="margin:0 0 20px;font-size:26px">Verify your email address</h1>
<p>Hello {{.Name}},</p><p>Confirm your email address to activate your Zeexco account.</p>
<p style="margin:28px 0"><a href="{{.URL}}" style="background:#172033;color:#ffffff;text-decoration:none;padding:13px 20px;border-radius:8px;display:inline-block">Verify email address</a></p>
<p style="font-size:14px;color:#596275">If the button does not work, copy this address into your browser:</p>
<p style="font-size:14px;word-break:break-all"><a href="{{.URL}}">{{.URL}}</a></p>
</td></tr></table></td></tr></table></body></html>`))

var welcomeEmailTemplate = template.Must(template.New("welcome").Parse(`<!doctype html>
<html><body style="margin:0;background:#f5f7fa;font-family:Arial,sans-serif;color:#172033">
<table role="presentation" width="100%" cellspacing="0" cellpadding="0"><tr><td style="padding:32px 16px">
<table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="max-width:600px;margin:auto;background:#ffffff;border-radius:12px">
<tr><td style="padding:36px"><h1 style="margin:0 0 20px;font-size:26px">Welcome to Zeexco</h1>
<p>Hello {{.Name}},</p><p>Your account is ready.</p>
<p>Zeexco helps you manage your business and create a stronger foundation for growth.</p>
<p style="margin:28px 0"><a href="{{.URL}}" style="background:#172033;color:#ffffff;text-decoration:none;padding:13px 20px;border-radius:8px;display:inline-block">Log in and get started</a></p>
<p style="font-size:14px;color:#596275">We are glad to have you with us.</p>
</td></tr></table></td></tr></table></body></html>`))

func renderVerificationEmail(firstName, link string) (RenderedEmail, error) {
	htmlBody, err := executeEmailTemplate(verificationEmailTemplate, firstName, link)
	if err != nil {
		return RenderedEmail{}, err
	}
	return RenderedEmail{
		Purpose: EmailPurposeSecurity, Subject: "Verify your Zeexco email address", HTMLBody: htmlBody,
		TextBody: fmt.Sprintf("Hello %s,\n\nConfirm your email address to activate your Zeexco account:\n%s\n", firstName, link),
	}, nil
}

func renderWelcomeEmail(firstName, loginURL string) (RenderedEmail, error) {
	htmlBody, err := executeEmailTemplate(welcomeEmailTemplate, firstName, loginURL)
	if err != nil {
		return RenderedEmail{}, err
	}
	return RenderedEmail{
		Purpose: EmailPurposeLifecycle, Subject: "Welcome to Zeexco", HTMLBody: htmlBody,
		TextBody: fmt.Sprintf("Hello %s,\n\nWelcome to Zeexco. Your account is ready.\n\nLog in and get started: %s\n", firstName, loginURL),
	}, nil
}

func executeEmailTemplate(value *template.Template, firstName, targetURL string) (string, error) {
	if value == nil || strings.TrimSpace(firstName) == "" || strings.TrimSpace(targetURL) == "" {
		return "", errors.New("complete email template input is required")
	}
	var output bytes.Buffer
	if err := value.Execute(&output, struct{ Name, URL string }{firstName, targetURL}); err != nil {
		return "", fmt.Errorf("render email template: %w", err)
	}
	return output.String(), nil
}
