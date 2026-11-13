CREATE TABLE whatsapp_onboarding_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider VARCHAR(50) NOT NULL,
    provider_user_id VARCHAR(255) NOT NULL,
    business_phone_number_id VARCHAR(255) NOT NULL,
    token_hash BYTEA NOT NULL,
    status VARCHAR(40) NOT NULL DEFAULT 'REGISTRATION_PENDING',
    expires_at TIMESTAMPTZ NOT NULL,
    welcome_delivery_started_at TIMESTAMPTZ,
    welcome_sent_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    completion_delivery_started_at TIMESTAMPTZ,
    completion_sent_at TIMESTAMPTZ,
    linked_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    linked_business_id UUID REFERENCES businesses(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT whatsapp_onboarding_provider_check
        CHECK (provider = 'META_WHATSAPP'),
    CONSTRAINT whatsapp_onboarding_provider_user_not_empty
        CHECK (btrim(provider_user_id) <> ''),
    CONSTRAINT whatsapp_onboarding_business_phone_not_empty
        CHECK (btrim(business_phone_number_id) <> ''),
    CONSTRAINT whatsapp_onboarding_token_hash_length
        CHECK (octet_length(token_hash) = 32),
    CONSTRAINT whatsapp_onboarding_token_hash_unique UNIQUE (token_hash),
    CONSTRAINT whatsapp_onboarding_status_check
        CHECK (status IN ('REGISTRATION_PENDING', 'FIRST_PRODUCT_PENDING', 'EXPIRED')),
    CONSTRAINT whatsapp_onboarding_expiration_check CHECK (expires_at > created_at),
    CONSTRAINT whatsapp_onboarding_completion_check CHECK (
        (status = 'FIRST_PRODUCT_PENDING'
            AND completed_at IS NOT NULL
            AND linked_user_id IS NOT NULL
            AND linked_business_id IS NOT NULL)
        OR
        (status <> 'FIRST_PRODUCT_PENDING'
            AND completed_at IS NULL
            AND linked_user_id IS NULL
            AND linked_business_id IS NULL)
    )
);

CREATE UNIQUE INDEX whatsapp_onboarding_one_pending_identity
ON whatsapp_onboarding_sessions(provider, provider_user_id)
WHERE status = 'REGISTRATION_PENDING';

CREATE INDEX idx_whatsapp_onboarding_linked_user
ON whatsapp_onboarding_sessions(linked_user_id)
WHERE linked_user_id IS NOT NULL;

CREATE INDEX idx_whatsapp_onboarding_expiration
ON whatsapp_onboarding_sessions(expires_at)
WHERE status = 'REGISTRATION_PENDING';
