# Zeexco Core

Zeexco Core is the deterministic commerce backend for the Zeexco platform.

## Run locally

Prerequisites:

- Go 1.26.2 or newer

Configuration is read from environment variables. The API defaults to the
`development` environment and port `8080`. `DATABASE_URL`,
`WHATSAPP_WEBHOOK_VERIFY_TOKEN`, and the Meta Developer app's
`META_APP_SECRET` are optional as a pair. Configure both to enable WhatsApp
webhook routes; leave both unset to run channel-independent browser
registration without Meta configuration.

## Database migrations

SQL migrations in `migrations/` are embedded in the Go binary. The established
`golang-migrate` mechanism records the current version and dirty state in
`schema_migrations`. Missing numbers are allowed; `000002` is an intentional
historical gap and does not prevent later versions from running.

The API runs pending migrations after connecting to PostgreSQL and before it
registers routes or starts listening. A PostgreSQL advisory lock serializes
multiple API instances. If inspection, preflight, or migration execution fails,
the transaction rolls back and the API exits rather than running against an
outdated schema.

To apply migrations explicitly after pulling new code:

```powershell
docker compose up -d postgres
$env:DATABASE_URL = "postgres://zeexco:local_dev_password@localhost:5433/zeexco_dev?sslmode=disable"
go run ./cmd/migrate
```

Use `go run ./cmd/migrate status` for a read-only summary of the relevant
tables, migration tracking, duplicate-email group count, and legacy active-user
count.

`docker compose up --build` also applies pending migrations automatically when
the API container starts. Rebuilding an image alone does not alter the database;
the migration runs when the rebuilt API starts.

For databases created before migration tracking existed, the runner validates
the known legacy tables, columns, constraints, and indexes before recording
their historical versions. It then applies only pending migrations. A partially
applied legacy migration is rejected for explicit repair rather than hidden
with `IF NOT EXISTS` statements.

Before `000006`, check for case-insensitive duplicate email groups if desired:

```sql
SELECT count(*) AS duplicate_email_groups
FROM (
    SELECT lower(email)
    FROM users
    WHERE email IS NOT NULL
    GROUP BY lower(email)
    HAVING count(*) > 1
) duplicates;
```

The runner performs this preflight automatically and reports only the number of
duplicate groups. It never deletes or prints the affected emails. Resolve any
duplicates deliberately, then rerun the migration command.

Migration `000006` adds the active-user password constraint as `NOT VALID`.
PostgreSQL therefore permits legacy `ACTIVE` rows that already lack a password
hash, while enforcing the constraint for new or subsequently updated rows. The
runner reports their count. They cannot use password login and require an
explicit account-recovery or administrative remediation process; migrations do
not invent passwords.

Web authentication fails closed unless an Ed25519 signing-key pair is
configured. Generate a local pair without committing it:

```powershell
openssl genpkey -algorithm Ed25519 -out auth-private.pem
openssl pkey -in auth-private.pem -pubout -out auth-public.pem
$env:AUTH_JWT_PRIVATE_KEY_BASE64 = [Convert]::ToBase64String([IO.File]::ReadAllBytes("auth-private.pem"))
$env:AUTH_JWT_PUBLIC_KEY_BASE64 = [Convert]::ToBase64String([IO.File]::ReadAllBytes("auth-public.pem"))
```

Keep `auth-private.pem` and its environment value outside Git and secret logs.
The defaults are issuer `zeexco`, audience `zeexco-api`, a 15-minute access
token, and a 30-day rotating refresh session. A separately deployed verifier
can use only the public key; it does not need access to the signing key.

Persisted user IDs use PostgreSQL UUIDs. Go models and API responses represent
those UUIDs in canonical hyphenated text form. The earlier temporary
`usr_<hex>` registration identifier is no longer used; any future public user
identifier should be a separate field rather than a replacement primary key.

## Register a merchant business

`POST /api/v1/auth/register` atomically creates a `PENDING` user, active
business, `OWNER` membership, email-verification record, and durable email
delivery job. Email is normalized to lowercase, uniqueness is enforced by
PostgreSQL, and passwords are stored only as bcrypt hashes. The account cannot
log in until the email address is verified.

```json
{
  "first_name": "Ada",
  "last_name": "Lovelace",
  "business_name": "Analytical Commerce",
  "email": "ada@example.com",
  "password": "correct horse battery staple"
}
```

The `202 Accepted` response contains the pending user, business, and
`"next_step":"VERIFY_EMAIL"`. A database-backed worker sends the activation
message through Resend outside the request path. Failed and interrupted jobs
are retried with exponential backoff; Resend receives a stable idempotency key
so retries do not intentionally duplicate mail.

The activation page reads the signed `token` query parameter and sends
`{"token":"..."}` to `POST /api/v1/auth/verify-email`. A valid, unexpired,
single-use token changes the user to `ACTIVE`; the frontend can then redirect
to login. Registration does not create a WhatsApp identity or auto-login.
The authoritative local contract is `docs/api/openapi.yaml`.

WhatsApp-first registration is intentionally separate. After validating the
opaque onboarding token, the frontend calls
`POST /api/v1/whatsapp/onboarding/register` with required `first_name`,
`last_name`, `business_name`, and `onboarding_token`; `email` is optional and
no password is accepted. The linked WhatsApp identity is resolved only from
the server-side onboarding session. A WhatsApp-only user stores `NULL` for
both an omitted email and its password hash, while the identity link is
created in the same transaction.

Future WhatsApp passwordless login should exchange a separate short-lived,
single-use credential for the existing refresh session and Ed25519 access JWT.
It must resolve to the same canonical user UUID and `Principal{UserID}`; access
JWTs must never be placed directly in WhatsApp CTA URLs.

## Browser authentication

`POST /api/v1/auth/login` accepts email and password. It returns the short-lived
access JWT in JSON and sets the opaque refresh credential only as an HttpOnly
cookie. The frontend should keep the access token in memory and send it as
`Authorization: Bearer <token>`; it should not put either credential in
`localStorage`. `POST /api/v1/auth/refresh` rotates the cookie and returns a new
access JWT. `GET /api/v1/auth/me` requires the Bearer token and returns the user
plus current business membership roles.

The refresh cookie defaults to `Secure`, `HttpOnly`, `SameSite=Lax`, and path
`/api/v1/auth`. Production must retain `AUTH_REFRESH_COOKIE_SECURE=true`. For
local HTTP development only, explicitly set
`AUTH_REFRESH_COOKIE_SECURE=false`; `SameSite=None` is rejected unless Secure is
enabled. The cookie name, domain, SameSite mode, and token lifetimes are
configurable through the variables in `.env.example`.

`POST /api/v1/auth/logout` revokes the presented refresh session and clears the
cookie. A previously issued access JWT is stateless and remains valid until its
short expiry (15 minutes by default); there is intentionally no access-token
denylist in this phase.

In PowerShell:

```powershell
$env:APP_ENV = "development"
$env:PORT = "8080"
$env:DATABASE_URL = "postgres://zeexco:local_dev_password@localhost:5433/zeexco_dev?sslmode=disable"
$env:AUTH_JWT_PRIVATE_KEY_BASE64 = "<base64-pkcs8-private-pem>"
$env:AUTH_JWT_PUBLIC_KEY_BASE64 = "<base64-pkix-public-pem>"
$env:AUTH_REFRESH_COOKIE_SECURE = "false" # local HTTP only
$env:WHATSAPP_WEBHOOK_VERIFY_TOKEN = "your-generated-token"
$env:META_APP_SECRET = "your-meta-app-secret"
go run ./cmd/api
```

Generate a cryptographically secure webhook verification token in PowerShell:

```powershell
$verifyToken = [Convert]::ToHexString(
    [Security.Cryptography.RandomNumberGenerator]::GetBytes(32)
).ToLowerInvariant()
$verifyToken
```

Store that value in your local environment or uncommitted `.env` file as
`WHATSAPP_WEBHOOK_VERIFY_TOKEN`. Use the same value in Meta's webhook
configuration. This verification token is a Zeexco-chosen handshake secret; it
is not the WhatsApp Cloud API access token and it is not the Meta App Secret.

Test the verification endpoint while the API is running:

```powershell
$challenge = "zeexco-test-challenge"
Invoke-WebRequest -UseBasicParsing `
    -Uri "http://localhost:8080/webhooks/whatsapp?hub.mode=subscribe&hub.verify_token=$([uri]::EscapeDataString($env:WHATSAPP_WEBHOOK_VERIFY_TOKEN))&hub.challenge=$([uri]::EscapeDataString($challenge))"
```

The successful response has status `200`, content type
`text/plain; charset=utf-8`, and a body exactly equal to
`zeexco-test-challenge`. Invalid or missing verification parameters return
`403 Forbidden`.

## Configure Meta webhook verification

Once the API is available at a public HTTPS URL:

1. Open the Meta Developer Dashboard for the Zeexco app.
2. Open **WhatsApp > Configuration** and choose **Configure a webhook**.
3. Set the callback URL to
   `https://<public-host>/webhooks/whatsapp`.
4. Set the verify token to the exact value configured as
   `WHATSAPP_WEBHOOK_VERIFY_TOKEN` on the API server.
5. Save the webhook configuration. Meta sends the verification request and
   accepts the callback only when the API returns its challenge exactly.

The verify token is only the shared webhook-handshake secret. Do not enter a
WhatsApp Cloud API access token or the Meta App Secret in this field.

## Test a signed POST webhook locally

`META_APP_SECRET` is the app secret from the Meta Developer Dashboard. It is
not the GET verification token and it is not a WhatsApp Cloud API access
token. Keep it in the process environment or an uncommitted `.env` file.

The following PowerShell creates a test signature from the exact ASCII JSON
bytes sent by `curl.exe`. It reads the secret from the environment and does
not print it:

```powershell
$payload = '{"object":"whatsapp_business_account","entry":[]}'
$payloadBytes = [Text.Encoding]::UTF8.GetBytes($payload)
$secretBytes = [Text.Encoding]::UTF8.GetBytes($env:META_APP_SECRET)
$hmac = [Security.Cryptography.HMACSHA256]::new($secretBytes)

try {
    $signature = [Convert]::ToHexString(
        $hmac.ComputeHash($payloadBytes)
    ).ToLowerInvariant()
} finally {
    $hmac.Dispose()
}

curl.exe -i `
    -X POST "http://localhost:8080/webhooks/whatsapp" `
    -H "Content-Type: application/json" `
    -H "X-Hub-Signature-256: sha256=$signature" `
    --data-binary $payload
```

A valid signed request returns `200 OK`. Changing the payload after calculating
the signature returns `403 Forbidden`.

After signature verification, WhatsApp message envelopes are parsed into typed
records. The initial parser supports text messages, interactive button replies,
interactive list replies, and template quick-reply buttons. Other nonempty
message types are acknowledged as unsupported without being converted to text.
Status-only events and empty entry lists are valid and produce zero parsed
messages. Parsing does not persist data or authorize a business action.

## Durable WhatsApp ingestion and identity

Authenticated, valid incoming messages are stored in `whatsapp_messages`
before the webhook is acknowledged. The Meta message ID has a PostgreSQL
unique constraint, and insertion uses `ON CONFLICT DO NOTHING`, so repeated
Meta deliveries return `200 OK` without creating a second row.

Stored content is normalized rather than the raw webhook payload. Depending on
message type, it contains text, interactive button/list identifiers and titles,
or template-button payload and text. Secrets, signatures, authorization
headers, and raw provider payloads are never stored.

Identity resolution is separate from message storage. The domain API resolves
`provider + provider_user_id`; its current WhatsApp adapter reads the existing
`whatsapp_identities` link table. An unknown sender remains an unregistered
channel contact: the message is stored, but no `users`, businesses, identities,
sessions, or registration records are created. Web/App registration does not
require a messaging identity, and another provider adapter can be added later
without changing callers of the identity resolver.

Message receipt and processing completion are intentionally separate. A new
row starts as `RECEIVED`. Identity resolution runs while the row is locked in a
PostgreSQL transaction and completion changes it to `IDENTITY_RESOLVED`. A
registered result stores only the resolved user UUID; a completed unknown
result stores `NULL`. If resolution fails, the transaction rolls back and the
durable row remains `RECEIVED`, so a duplicate delivery resumes it. Completed
duplicates do not resolve identity again. The unknown result applies only to
that message; a later message always performs a new lookup.

## PostgreSQL integration tests

The integration suite uses the production migration runner against an isolated schema
inside a dedicated test database. It refuses database names that do not contain
`test`, never calls Meta, and drops only its randomly named test schema.

```powershell
docker compose --profile integration up -d postgres-test
$env:TEST_DATABASE_URL = "postgres://zeexco_test:local_test_password@localhost:5434/zeexco_test?sslmode=disable"
go test -count=1 -tags=integration ./internal/features/authentication/repositories ./internal/features/whatsapp/repositories ./internal/features/whatsapp/onboarding
docker compose --profile integration stop postgres-test
Remove-Item Env:TEST_DATABASE_URL -ErrorAction SilentlyContinue
```

## WhatsApp secure web onboarding

When the verified webhook receives a message from an unknown Meta WhatsApp
identity, Zeexco creates one active `REGISTRATION_PENDING` onboarding session.
When `WHATSAPP_MERCHANT_REGISTRATION_FLOW_ID` is configured, Zeexco sends a
Cloud API `interactive` `flow` message whose **Get started** button opens the
Zeexco merchant-registration Flow inside WhatsApp. The server-generated
onboarding token is transported only as Meta's non-visible `flow_token`.

If the Flow ID is not configured, or Meta rejects the Flow message, Zeexco
sends the existing `interactive` `cta_url` fallback. That button opens
`/register?token=<opaque-token>` on the configured public frontend HTTPS origin.
Both transports use the same claimed session and token. Repeated webhook
deliveries do not create additional sessions or uncontrolled messages.

The token contains 256 random bits generated with `crypto/rand`. Only its
SHA-256 digest is stored in `whatsapp_onboarding_sessions`; the raw token is
sent only in the CTA URL, is not logged, expires after 30 minutes by default,
and can complete registration once. The form never asks for or trusts a phone
number. A successful transaction atomically creates the user, business, OWNER
membership, Meta WhatsApp identity, audit events, and transitions the session
to `FIRST_PRODUCT_PENDING`.

If the normalized email already exists, the request returns a safe conflict.
It does not link the WhatsApp identity; authenticated account linking is a
future flow. After commit, Zeexco sends an interactive reply button with ID
`CREATE_FIRST_PRODUCT`. Delivery claims are persisted, so a temporary outbound
failure can be retried when the success page is reopened or the linked user
messages again. Clicking the button currently returns only `Product setup is
coming next.` Product creation is intentionally outside this task.

Configure these values in addition to the existing webhook and Cloud API
credentials:

```powershell
$env:PUBLIC_FRONTEND_BASE_URL = "https://www.zeexco.com"
$env:WHATSAPP_ONBOARDING_RETURN_URL = "https://wa.me/"
$env:WHATSAPP_ONBOARDING_TOKEN_TTL = "30m"
$env:WHATSAPP_MERCHANT_REGISTRATION_FLOW_ID = "<published-meta-flow-id>"
```

The Flow JSON source is
`docs/flows/merchant-registration.flow.json`. It contains one minimal
`REGISTRATION` screen with required first name, last name, and business name,
optional email, and a `data_exchange` footer. Upload this asset to the Meta Flow
that owns the configured ID and keep its endpoint set to
`POST /api/v1/whatsapp/flows`. The Flow ID is environment-specific and must not
be hardcoded. Leave it blank to force browser fallback.

The public frontend base URL and return URL must be absolute HTTPS URLs. The
frontend origin is also the exact CORS allow-origin. Leaving it unset disables
onboarding while preserving browser auth and inbound webhook processing. The
API automatically applies migrations through `000009`
on startup. To apply it explicitly:

```powershell
$env:DATABASE_URL = "postgres://zeexco:local_dev_password@localhost:5433/zeexco_dev?sslmode=disable"
go run ./cmd/migrate
```

For a Meta test-environment end-to-end check:

1. Expose the API over HTTPS and set the public base URL to that origin.
2. In Meta WhatsApp Configuration, keep the callback at
   `https://<public-host>/webhooks/whatsapp`, use the configured verify token,
   and subscribe the app to the `messages` field.
3. Use the same app's App Secret for webhook HMAC verification and configure a
   valid Cloud API access token, sender phone-number ID, and supported Graph API
   version. Add the test recipient in Meta's API Setup screen.
4. Restart the API, then send `Hi Zeexco` from a WhatsApp number with no row in
   `whatsapp_identities`.
5. Tap **Get started**, let the frontend validate the token, submit the five
   registration fields, and confirm the automatic **Create first product** invitation.
6. Verify one user, one business, one OWNER membership, one WhatsApp identity,
   one `FIRST_PRODUCT_PENDING` session, and the onboarding audit events in
   PostgreSQL.

Live testing is blocked if the public HTTPS route cannot reach both
`/webhooks/whatsapp` and the frontend `/register` page, the Meta test recipient is not
authorized, the access token is expired, or the Cloud API message falls outside
Meta's allowed customer-service window without an approved template.

## Test through Cloudflare Tunnel and Meta

1. Set `META_APP_SECRET` in the API container environment to the secret for
   the same Meta Developer app that owns the webhook.
2. Restart the API container and confirm the existing Cloudflare Tunnel routes
   its public HTTPS hostname to the API on port `8080`.
3. Keep the callback URL configured as
   `https://<public-host>/webhooks/whatsapp`.
4. In **Meta Developer Dashboard > WhatsApp > Configuration**, find the
   subscribed webhook field (for example, `messages`) and choose **Test**.
5. Confirm Meta receives `200 OK`. A dashboard-generated message event produces
   a safe log similar to:

   ```text
   whatsapp webhook accepted object=whatsapp_business_account outcome=ok message_count=1 message_types=text:1,interactive:0,button:0,unsupported:0
   ```

   The log must not contain the body, signature, secret, phone number, WhatsApp
   IDs, message IDs, or message text.

If Meta receives `403`, first confirm the container uses the App Secret from
the same Meta app. If it receives `413`, the request exceeded the current
1 MiB HTTP-boundary limit.

## Send a development WhatsApp text message

Outbound messaging is configured separately from inbound webhooks. The API can
continue receiving webhooks without outbound credentials because it does not
automatically send replies. The outbound client validates these variables only
when it is constructed:

- `WHATSAPP_ACCESS_TOKEN`: Meta access token with permission to send WhatsApp
  messages. Treat it as a secret.
- `WHATSAPP_PHONE_NUMBER_ID`: the sender phone-number ID shown in Meta's
  WhatsApp API setup, not the displayed telephone number.
- `META_GRAPH_API_VERSION`: a Graph API version currently supported by the
  configured Meta app.

The repository includes a development-only command rather than an
unauthenticated HTTP test endpoint. In a PowerShell session, set the values
without writing them into the repository:

```powershell
$env:WHATSAPP_ACCESS_TOKEN = "<meta-access-token>"
$env:WHATSAPP_PHONE_NUMBER_ID = "<meta-test-phone-number-id>"
$env:META_GRAPH_API_VERSION = "<supported-version-from-meta>"
$env:WHATSAPP_TEST_RECIPIENT = "<authorized-test-recipient-wa-id>"

go run ./cmd/whatsapp-send
```

The command sends exactly `Hello from Zeexco`. It logs only the operation
outcome and does not print the access token, recipient, message body, request
body, or provider response body.

Before running it, confirm in Meta's WhatsApp API setup that:

1. The phone-number ID belongs to the test number associated with the app.
2. The recipient has been added and verified as an allowed test recipient.
3. The access token is valid and authorized for WhatsApp messaging.
4. The chosen Graph API version is enabled and supported for the app.
5. The recipient is eligible for a normal text message under Meta's current
   messaging-window rules; otherwise Meta may require an approved template.

Remove the credentials from the shell after testing:

```powershell
Remove-Item Env:WHATSAPP_ACCESS_TOKEN -ErrorAction SilentlyContinue
Remove-Item Env:WHATSAPP_PHONE_NUMBER_ID -ErrorAction SilentlyContinue
Remove-Item Env:META_GRAPH_API_VERSION -ErrorAction SilentlyContinue
Remove-Item Env:WHATSAPP_TEST_RECIPIENT -ErrorAction SilentlyContinue
```

## Verify

```powershell
go test ./...
go vet ./...
```

## Meta WhatsApp Flow data endpoint

`POST /api/v1/whatsapp/flows` implements Meta Flow Data Exchange protocol 3.0.
It verifies `X-Hub-Signature-256` with `META_APP_SECRET`, decrypts the request
AES key with RSA-OAEP/SHA-256, decrypts Flow data using AES-128-GCM, and returns
base64 encrypted `text/plain` using the same AES key and the bitwise-inverted
request IV. The encrypted `ping` action returns `status: active` without any
database write.

Configure:

- `WHATSAPP_FLOW_PRIVATE_KEY`: a 2048-bit or stronger RSA private key in
  PKCS#1 or PKCS#8 PEM. This remains only on the API host.
- `WHATSAPP_FLOW_PRIVATE_KEY_PASSPHRASE`: required only for an encrypted,
  traditional PKCS#1 PEM.
- `META_APP_SECRET`: the same Meta app secret used to verify webhook and Flow
  request signatures.

Generate an encrypted local key pair without committing either file:

```powershell
openssl genrsa -traditional -aes256 -passout pass:"<strong-passphrase>" -out whatsapp-flow-private.pem 2048
openssl rsa -in whatsapp-flow-private.pem -passin pass:"<strong-passphrase>" -pubout -out whatsapp-flow-public.pem
```

Store the private PEM and passphrase in the deployment secret manager. Submit
only `whatsapp-flow-public.pem` to Meta for the selected WhatsApp phone number.
Never commit a production private key or its passphrase. Configure Meta's
endpoint URI as `https://<public-api-host>/api/v1/whatsapp/flows`; a temporary
Cloudflare hostname belongs only in Meta configuration, never application code.

The future outbound Flow message must set `flow_token` to the existing opaque
onboarding token. Until outbound Flow sending is implemented and that binding
is exercised, keep the existing URL CTA/browser registration fallback enabled.
