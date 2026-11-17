CREATE TABLE whatsapp_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    provider_message_id VARCHAR(255) NOT NULL,
    sender_provider_id VARCHAR(255) NOT NULL,
    business_phone_number_id VARCHAR(255) NOT NULL,

    message_type VARCHAR(50) NOT NULL,
    normalized_content JSONB NOT NULL,
    processing_status VARCHAR(20) NOT NULL DEFAULT 'RECEIVED',

    provider_timestamp TIMESTAMPTZ,
    received_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT whatsapp_messages_provider_message_id_not_empty
        CHECK (btrim(provider_message_id) <> ''),

    CONSTRAINT whatsapp_messages_sender_provider_id_not_empty
        CHECK (btrim(sender_provider_id) <> ''),

    CONSTRAINT whatsapp_messages_business_phone_number_id_not_empty
        CHECK (btrim(business_phone_number_id) <> ''),

    CONSTRAINT whatsapp_messages_message_type_not_empty
        CHECK (btrim(message_type) <> ''),

    CONSTRAINT whatsapp_messages_normalized_content_object
        CHECK (jsonb_typeof(normalized_content) = 'object'),

    CONSTRAINT whatsapp_messages_processing_status_check
        CHECK (processing_status IN ('RECEIVED')),

    CONSTRAINT whatsapp_messages_provider_message_id_unique
        UNIQUE (provider_message_id)
);

CREATE INDEX idx_whatsapp_messages_sender_provider_id
ON whatsapp_messages(sender_provider_id);

CREATE INDEX idx_whatsapp_messages_business_phone_number_id
ON whatsapp_messages(business_phone_number_id);

CREATE INDEX idx_whatsapp_messages_received_at
ON whatsapp_messages(received_at);
