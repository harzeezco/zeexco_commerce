# Verified WhatsApp browser signup

1. A merchant sends a message to the configured Zico WhatsApp account.
2. The signed webhook creates a trusted onboarding session. The welcome CTA
   opens `${PUBLIC_FRONTEND_BASE_URL}/register?token=<opaque-token>`.
3. The frontend validates the token with
   `GET /api/v1/whatsapp/onboarding/validate?token=...`. Never collect or submit
   provider, WhatsApp ID, sender phone, user ID, or business ID from the browser.
4. Preserve the token in form state through validation and submit:

```http
POST /api/v1/auth/register
Content-Type: application/json
```

```json
{
  "first_name": "Ada",
  "last_name": "Lovelace",
  "business_name": "Ada Store",
  "email": "ada@example.com",
  "password": "Securepass1!",
  "onboarding_token": "<token from the original Get started link>"
}
```

The dedicated `/api/v1/whatsapp/onboarding/register` endpoint accepts the same
fields and requires the token. Ordinary web signup omits `onboarding_token`.
Passwords follow the canonical 8-character minimum, letter/digit/symbol rules,
and 72-byte maximum. Never trim passwords or send them in URLs.

5. HTTP 202 returns only `status_code` and `message`. Show Check your email.
   The account is PENDING and the source session is EMAIL_VERIFICATION_PENDING.
   No browser session cookies or WhatsApp identity are created yet. Duplicate,
   expired, or unknown onboarding tokens fail without creating an account.
6. The email links to the existing frontend verification route. This contains
   the signed email-verification token, which is different from the original
   onboarding token. POST it as `{"token":"..."}` to `/api/v1/auth/verify-email`.
7. Ordinary web signup returns HTTP 204. A verified WhatsApp signup returns:

```json
{
  "status_code": 200,
  "message": "Account activated successfully. Continue in WhatsApp.",
  "next_step": "CONTINUE_IN_WHATSAPP",
  "redirect_url": "https://wa.me/15551892941?text=Continue"
}
```

Handle both 200 and 204 as successful activation. For CONTINUE_IN_WHATSAPP, show
a button linking to the returned URL and tell the user to tap Send. The backend
builds this URL from trusted `WHATSAPP_ONBOARDING_RETURN_URL`; never accept a
browser-supplied destination. Do not store onboarding tokens in analytics,
logs, return URLs, or the WhatsApp message. Configure the public number rather
than Meta's phone-number ID. Replace the development test number at production
launch. If redirect_url is omitted, instruct the user to return to the chat.

8. Activation commits the ACTIVE user, trusted WhatsApp identity link, welcome
   email job, FIRST_PRODUCT_PENDING state, and audit records together.
9. The newly received Continue message resolves the user and sends the existing
   deterministic Create first product invitation. This does not implement the
   subsequent product creation wizard; its current button handler remains a
   placeholder for the Product foundation work.

Reopening an onboarding link after signup shows Check your email until activation.
Its API validation returns ONBOARDING_VERIFICATION_PENDING. Repeated activation
tokens return EMAIL_VERIFICATION_CONSUMED without duplicating identity links or
welcome jobs. Existing-email accounts are rejected; authenticated account
linking is separate work and must never happen solely through email matching.
