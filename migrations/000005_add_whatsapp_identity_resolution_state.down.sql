DROP INDEX IF EXISTS idx_whatsapp_messages_resolved_user_id;

ALTER TABLE whatsapp_messages
    DROP CONSTRAINT whatsapp_messages_identity_resolution_check,
    DROP CONSTRAINT whatsapp_messages_processing_status_check,
    DROP COLUMN resolved_user_id;

ALTER TABLE whatsapp_messages
    ADD CONSTRAINT whatsapp_messages_processing_status_check
        CHECK (processing_status IN ('RECEIVED'));
