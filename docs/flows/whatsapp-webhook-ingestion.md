# WhatsApp webhook ingestion

After the request-size limit and `X-Hub-Signature-256` verification succeed, the
handler validates that the body is a JSON object with an `object` field. Valid
JSON deliveries are stored unchanged as JSONB in `whatsapp_webhook_events`
before message parsing begins. Malformed JSON, invalid signatures, and oversized
bodies are never stored.

Each HTTP delivery creates a webhook-event row. Meta does not provide a reliable
identifier for the whole envelope, and one envelope may contain zero or many
messages or statuses, so the system does not invent an envelope ID or collapse
raw delivery evidence. Idempotency is enforced downstream by Meta's message ID
on `whatsapp_messages`. A duplicate delivery therefore remains auditable but
cannot create another normalized message or repeat its onboarding side effect.

After the raw event transaction commits, the webhook returns `200 OK`. A
PostgreSQL-backed worker claims events with `FOR UPDATE SKIP LOCKED`, so multiple
API instances can process different deliveries without Redis or an in-memory
queue. Events transition `RECEIVED -> PROCESSING -> PROCESSED` or
`RECEIVED -> PROCESSING -> FAILED`. `attempt_count` increments when processing
starts. Failed or stale processing claims are retried after the configured delay
up to the configured attempt limit. Failure records contain a bounded internal
error category and safe message, not the raw customer payload. Normalized messages reference their
first source event through nullable `source_webhook_event_id`; the column is
nullable so historical messages remain valid, and one event may source many
messages.

The raw event insert commits independently of normalization and outbound calls.
Normalized message inserts also commit before identity resolution and onboarding
delivery. A durable-storage failure returns non-2xx. A signed envelope with an
invalid downstream message shape is acknowledged after storage and later marked
`FAILED` by the worker. Unsupported valid objects are recorded, acknowledged,
and later marked `PROCESSED`. A Meta outbound CTA rejection releases the
onboarding delivery claim, leaves both inbound records durable, and returns 200
so Meta does not redeliver an accepted inbound message merely because an
outbound dependency failed.
