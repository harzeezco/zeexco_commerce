CREATE TABLE whatsapp_webhook_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider VARCHAR(50) NOT NULL,
    provider_account_id VARCHAR(255),
    phone_number_id VARCHAR(255),
    event_type VARCHAR(100) NOT NULL,
    payload JSONB NOT NULL,
    processing_status VARCHAR(20) NOT NULL DEFAULT 'RECEIVED',
    received_at TIMESTAMPTZ NOT NULL,
    processing_started_at TIMESTAMPTZ,
    processed_at TIMESTAMPTZ,
    attempt_count INTEGER NOT NULL DEFAULT 0,
    last_error_code VARCHAR(100),
    last_error_message VARCHAR(512),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT whatsapp_webhook_events_provider_check
        CHECK (provider = 'META_WHATSAPP'),
    CONSTRAINT whatsapp_webhook_events_event_type_not_empty
        CHECK (btrim(event_type) <> ''),
    CONSTRAINT whatsapp_webhook_events_payload_object
        CHECK (jsonb_typeof(payload) = 'object'),
    CONSTRAINT whatsapp_webhook_events_processing_status_check
        CHECK (processing_status IN ('RECEIVED', 'PROCESSING', 'PROCESSED', 'FAILED')),
    CONSTRAINT whatsapp_webhook_events_attempt_count_check
        CHECK (attempt_count >= 0),
    CONSTRAINT whatsapp_webhook_events_lifecycle_check
        CHECK (
            (processing_status = 'RECEIVED'
                AND processing_started_at IS NULL AND processed_at IS NULL)
            OR (processing_status = 'PROCESSING'
                AND processing_started_at IS NOT NULL AND processed_at IS NULL)
            OR (processing_status IN ('PROCESSED', 'FAILED')
                AND processing_started_at IS NOT NULL AND processed_at IS NOT NULL)
        )
);

CREATE INDEX idx_whatsapp_webhook_events_status_received
ON whatsapp_webhook_events(processing_status, received_at);

CREATE INDEX idx_whatsapp_webhook_events_provider_account
ON whatsapp_webhook_events(provider_account_id)
WHERE provider_account_id IS NOT NULL;

ALTER TABLE whatsapp_messages
    ADD COLUMN source_webhook_event_id UUID NULL
        REFERENCES whatsapp_webhook_events(id) ON DELETE RESTRICT;

CREATE INDEX idx_whatsapp_messages_source_webhook_event
ON whatsapp_messages(source_webhook_event_id)
WHERE source_webhook_event_id IS NOT NULL;
