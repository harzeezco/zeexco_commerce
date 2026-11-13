CREATE TABLE whatsapp_identities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    user_id UUID NOT NULL
        REFERENCES users(id)
        ON DELETE CASCADE,

    provider VARCHAR(50) NOT NULL
        CHECK (provider = 'META_WHATSAPP'),

    provider_user_id VARCHAR(255) NOT NULL,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT whatsapp_provider_user_id_not_empty
        CHECK (btrim(provider_user_id) <> ''),

    CONSTRAINT whatsapp_identity_unique
        UNIQUE (provider, provider_user_id)
);

CREATE INDEX idx_whatsapp_identities_user_id
ON whatsapp_identities(user_id);