ALTER TABLE whatsapp_messages
    DROP CONSTRAINT whatsapp_messages_processing_status_check;

ALTER TABLE whatsapp_messages
    ADD COLUMN resolved_user_id UUID NULL
        REFERENCES users(id),
    ADD CONSTRAINT whatsapp_messages_processing_status_check
        CHECK (processing_status IN ('RECEIVED', 'IDENTITY_RESOLVED')),
    ADD CONSTRAINT whatsapp_messages_identity_resolution_check
        CHECK (
            processing_status = 'IDENTITY_RESOLVED'
            OR resolved_user_id IS NULL
        );

CREATE INDEX idx_whatsapp_messages_resolved_user_id
ON whatsapp_messages(resolved_user_id)
WHERE resolved_user_id IS NOT NULL;
