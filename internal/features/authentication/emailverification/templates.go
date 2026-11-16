package emailverification

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"html/template"
	"strings"
	"time"
)

// A PNG rendition of the supplied SVG supports mail clients that cannot display SVG.
//
//go:embed assets/zico-logo.png
var zicoLogo []byte

const zicoLogoContentID = "zico-logo"

type RenderedEmail struct {
	Purpose  EmailPurpose
	Subject  string
	HTMLBody string
	TextBody string
}

//go:embed templates/account.html
var accountEmailHTML string

var accountEmailTemplate = template.Must(template.New("account").Parse(accountEmailHTML))

func renderVerificationEmail(firstName, link string, expiresAt, now time.Time) (RenderedEmail, error) {
	if expiresAt.IsZero() || !expiresAt.After(now) {
		return RenderedEmail{}, errors.New("future email verification expiry is required")
	}
	remaining := expiresAt.Sub(now)
	minutes := remaining / time.Minute
	if remaining%time.Minute != 0 {
		minutes++
	}
	expiry := fmt.Sprintf("%d minutes", minutes)
	if minutes == 1 {
		expiry = "1 minute"
	}
	htmlBody, err := executeEmailTemplateData(accountEmailTemplate, emailTemplateData{
		Name: firstName, URL: link, ExpiresIn: expiry, Year: now.UTC().Year(),
	})
	if err != nil {
		return RenderedEmail{}, err
	}
	return RenderedEmail{
		Purpose: EmailPurposeSecurity, Subject: "Activate your Zico account", HTMLBody: htmlBody,
		TextBody: fmt.Sprintf("Hi %s,\n\nConfirm your email address to activate your account and continue setting up your business on Zico.\n\nActivate my account:\n%s\n\nThis link expires in %s.\n\nIf you didn’t create a Zico account, you can safely ignore this email.\n\nZico helps everyday sellers spend less time on repetitive work and more time running their business.\n\n© %d Zico. All rights reserved.\nPrivacy: https://zeexco.com/privacy\nTerms: https://zeexco.com/terms\nSupport: support@zeexco.com\n", firstName, link, expiry, now.UTC().Year()),
	}, nil
}

func renderWelcomeEmail(firstName, loginURL string) (RenderedEmail, error) {
	htmlBody, err := executeEmailTemplateData(accountEmailTemplate, emailTemplateData{
		Name: firstName, URL: loginURL, Welcome: true, Year: time.Now().UTC().Year(),
	})
	if err != nil {
		return RenderedEmail{}, err
	}
	return RenderedEmail{
		Purpose: EmailPurposeLifecycle, Subject: "Welcome to Zico", HTMLBody: htmlBody,
		TextBody: fmt.Sprintf("A fresh start for your business.\n\nHi %s,\n\nYour account is active. Welcome to Zico — let's make the everyday work of running your business a little easier.\n\nA few small steps. A smoother day.\n\n01 Make it yours\nAdd your business name and details.\n\n02 Add your products\nBring your products and prices together.\n\n03 You're ready to sell\nStart managing your business with Zico.\n\nSet up my business: %s\n\nYou don't have to figure it all out at once. Need a hand getting started?\nContact support: support@zeexco.com\n\nZico helps everyday sellers spend less time on repetitive work and more time running their business.\n\nPrivacy: https://zeexco.com/privacy\nTerms: https://zeexco.com/terms\nSupport: support@zeexco.com\n\n© %d Zico. All rights reserved.\n", firstName, loginURL, time.Now().UTC().Year()),
	}, nil
}

type emailTemplateData struct {
	Name, URL, ExpiresIn string
	Year                 int
	Welcome              bool
}

func executeEmailTemplateData(value *template.Template, data emailTemplateData) (string, error) {
	if value == nil || strings.TrimSpace(data.Name) == "" || strings.TrimSpace(data.URL) == "" {
		return "", errors.New("complete email template input is required")
	}
	var output bytes.Buffer
	if err := value.Execute(&output, data); err != nil {
		return "", fmt.Errorf("render email template: %w", err)
	}
	return output.String(), nil
}
