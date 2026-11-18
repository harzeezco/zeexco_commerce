-- Refuse rollback while verified-browser registrations are in progress rather
-- than discarding their identity reservations.
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM whatsapp_onboarding_sessions WHERE status = 'EMAIL_VERIFICATION_PENDING') THEN
        RAISE EXCEPTION 'Cannot roll back while WhatsApp email verification is pending';
    END IF;
END; $$;
ALTER TABLE whatsapp_onboarding_sessions
    DROP CONSTRAINT whatsapp_onboarding_status_check,
    DROP CONSTRAINT whatsapp_onboarding_completion_check;
ALTER TABLE whatsapp_onboarding_sessions
    ADD CONSTRAINT whatsapp_onboarding_status_check CHECK (status IN ('REGISTRATION_PENDING', 'FIRST_PRODUCT_PENDING', 'EXPIRED')),
    ADD CONSTRAINT whatsapp_onboarding_completion_check CHECK (
        (status = 'FIRST_PRODUCT_PENDING' AND completed_at IS NOT NULL
            AND linked_user_id IS NOT NULL AND linked_business_id IS NOT NULL)
        OR (status <> 'FIRST_PRODUCT_PENDING' AND completed_at IS NULL
            AND linked_user_id IS NULL AND linked_business_id IS NULL)
    );
DROP INDEX whatsapp_onboarding_one_pending_identity;
CREATE UNIQUE INDEX whatsapp_onboarding_one_pending_identity
    ON whatsapp_onboarding_sessions(provider, provider_user_id) WHERE status = 'REGISTRATION_PENDING';
