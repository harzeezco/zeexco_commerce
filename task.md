We need to enable our existing WhatsApp Merchant Registration Flow for draft-mode testing while the Meta Flow is not yet published.

Context:

- Existing Flow ID: `2198301500826080`
- Existing registration screen: `REGISTRATION`
- Existing backend already creates genuine onboarding sessions and opaque onboarding tokens.
- Existing Cloud API integration already sends the Merchant Registration WhatsApp Flow.
- Existing encrypted Flow Data endpoint is working.
- Browser fallback must remain unchanged.
- Do NOT redesign registration.
- Do NOT create fake onboarding tokens.
- Do NOT change the Flow endpoint contract.
- Do NOT change registration business logic.
- Do NOT start Product work.

Goal:

Allow the existing WhatsApp Flow message to run in Meta draft/test mode through configuration so we can complete a real end-to-end WhatsApp registration test before the Flow is published.

Implementation requirements:

1. Inspect the existing WhatsApp configuration and Cloud API Flow-message implementation first.

2. Add a configuration value:

`WHATSAPP_FLOW_MODE`

Supported values should be limited to the modes actually supported by the WhatsApp Flow message API.

For our current test environment we want:

`WHATSAPP_FLOW_MODE=draft`

3. Pass the configured mode into the existing interactive WhatsApp Flow message in the correct Meta-supported location.

Do not guess the payload structure. Verify it against the existing implementation and Meta's expected Flow interactive-message schema.

4. Preserve all existing Flow parameters, including:

- Flow ID `2198301500826080` from configuration
- `flow_message_version`
- `flow_token`
- `flow_action`
- navigation to `REGISTRATION`
- existing CTA
- existing browser fallback

5. The `flow_token` MUST remain the genuine opaque Zeexco onboarding token generated from the server-side onboarding session.

Never replace it with a static development token.

The intended path remains:

Unknown WhatsApp sender
→ create onboarding session
→ generate genuine onboarding token
→ send Get started Flow CTA
→ Flow opens in draft/test mode
→ token travels through the Flow
→ encrypted Flow endpoint validates the real onboarding session
→ registration completes normally.

6. Configuration must fail safely.

Do not silently accept arbitrary Flow modes.

Use a sensible default that does not accidentally enable an unsafe production behavior. Follow the existing project's configuration conventions.

7. Update Docker Compose/environment propagation if `WHATSAPP_FLOW_MODE` needs to be passed into the API container.

8. Add/update focused tests covering:

- draft mode produces the expected Flow payload
- genuine flow_token is preserved
- existing REGISTRATION screen remains unchanged
- invalid configuration is handled safely
- existing fallback behavior remains intact

9. Do not alter unrelated code.

10. Run the relevant Go tests and `go vet ./...`.

At the end, report:

- files changed
- exact payload field added/changed
- how `WHATSAPP_FLOW_MODE` is parsed
- default behavior when it is missing
- test results
- vet result
- exact command/environment change I need to apply locally

Do not proceed to Product implementation.

Our immediate goal after this task is a real end-to-end draft WhatsApp registration test.