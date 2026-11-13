Update the existing Zeexco asynchronous email infrastructure so different email purposes use the appropriate sender identity.

## Context

Zeexco uses Resend for transactional email.

We now have two email addresses on our verified `zeexco.com` domain:

- `no-reply@zeexco.com`
- `hello@zeexco.com`

The intended responsibility is:

### Security/system email

Use:

`Zeexco <no-reply@zeexco.com>`

for emails such as:

- Email verification / account activation
- Password reset
- Password changed notifications
- Other future authentication/security notifications

### Lifecycle/human-friendly email

Use:

`Zeexco <hello@zeexco.com>`

for:

- Welcome to Zeexco
- Future merchant onboarding/lifecycle communication where a friendly sender is appropriate

Do not hardcode either address throughout application services.

## Configuration

Replace/generalize the existing single `RESEND_FROM_EMAIL` configuration if appropriate and introduce explicit configuration such as:

`RESEND_SECURITY_FROM_EMAIL="Zeexco <no-reply@zeexco.com>"`

`RESEND_LIFECYCLE_FROM_EMAIL="Zeexco <hello@zeexco.com>"`

Keep:

`RESEND_API_KEY`

`EMAIL_VERIFICATION_FRONTEND_URL=http://localhost:3000/verify-email`

`EMAIL_VERIFICATION_HMAC_SECRET_BASE64`

Update `.env.example`, configuration loading/validation, tests, and relevant documentation.

Never expose the Resend API key or HMAC secret to the Next.js frontend.

## Email verification

The existing email-verification flow should continue to use the durable asynchronous PostgreSQL outbox/worker architecture.

Verification emails must be sent from:

`Zeexco <no-reply@zeexco.com>`

Do not call Resend synchronously from the registration request.

Preserve the existing retry, idempotency, locking, token-expiry, rate-limiting, audit, and crash-recovery behavior.

## Welcome email

Add a Welcome to Zeexco email.

The welcome email must NOT be created when the initial registration request creates a PENDING user.

It should only be queued after email verification successfully transitions the user from PENDING to ACTIVE.

Desired flow:

Register
→ user PENDING
→ verification email job
→ worker
→ Resend from no-reply@zeexco.com
→ user clicks verification link
→ POST /api/v1/auth/verify-email
→ atomically transition user to ACTIVE
→ create durable welcome-email job
→ commit
→ worker sends welcome email asynchronously
→ Resend from hello@zeexco.com

The verification request must not wait for Resend.

If Resend is temporarily unavailable, successful account activation must remain successful and the welcome email must retry through the existing durable job mechanism.

Ensure duplicate verification requests, retries, worker crashes, or concurrent workers cannot produce duplicate welcome emails. Use database constraints/idempotency and the existing worker/outbox architecture rather than in-memory protection.

## Welcome email content

Create a clean, professional, responsive Zeexco welcome email.

Keep the language simple and merchant-focused.

Suggested intent:

"Welcome to Zeexco"

Confirm that their account is ready and briefly explain that Zeexco helps them manage and grow their business.

Provide a clear CTA to log in/get started.

Do not make claims about features that are not implemented yet.

Keep email templates separated from provider/infrastructure code so templates can evolve without changing the Resend client.

Provide both HTML and sensible plain-text content if the current email infrastructure supports it.

## Architecture

Resend must remain an infrastructure provider, not business logic.

Prefer a structure where application/domain code expresses the email purpose (verification, welcome, etc.) and the email infrastructure determines the configured sender/template.

Do not duplicate the existing worker system just for welcome emails. Extend/generalize the current durable email job infrastructure where appropriate.

Do not introduce Redis, Kafka, or another queue for this task. PostgreSQL remains the durable coordination mechanism.

## Frontend URL

The Zeexco frontend is Next.js and currently runs locally at:

`http://localhost:3000`

Email verification links should therefore use:

`http://localhost:3000/verify-email`

through the existing `EMAIL_VERIFICATION_FRONTEND_URL` configuration.

Do not hardcode localhost into application logic.

## Validation

Add/update tests covering at minimum:

1. Verification email uses the security/no-reply sender.
2. Welcome email uses the lifecycle/hello sender.
3. PENDING registration does not queue a welcome email.
4. Successful first verification queues exactly one welcome email.
5. Reusing a verification token does not queue another welcome email.
6. Concurrent verification cannot queue duplicate welcome emails.
7. Resend failure does not roll back successful verification.
8. Failed welcome-email jobs retry.
9. Worker crash/recovery preserves idempotency.
10. Configuration rejects malformed/missing required sender configuration where appropriate.
11. Existing authentication/email-verification tests continue to pass.

Run:

- full Go test suite
- relevant integration tests where available
- `go vet ./...`

Update architecture documentation and OpenAPI documentation if behavior/contracts changed.

Before making major architectural changes, inspect the existing implementation and extend the established patterns rather than replacing working email-verification infrastructure unnecessarily.

At completion, report:

- files changed
- database/schema changes
- environment variables required
- email job types introduced/changed
- how duplicate welcome emails are prevented
- tests executed and results
- anything requiring manual/live verification