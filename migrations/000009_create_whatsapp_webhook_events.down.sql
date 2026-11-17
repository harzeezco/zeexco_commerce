DROP INDEX IF EXISTS idx_whatsapp_messages_source_webhook_event;

ALTER TABLE whatsapp_messages
    DROP COLUMN source_webhook_event_id;

DROP TABLE whatsapp_webhook_events;
