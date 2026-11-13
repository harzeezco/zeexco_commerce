ALTER TABLE users
    ADD COLUMN password_hash VARCHAR(255),
    ADD CONSTRAINT users_password_hash_not_empty
        CHECK (password_hash IS NULL OR btrim(password_hash) <> ''),
    ADD CONSTRAINT users_active_password_hash_check
        CHECK (status <> 'ACTIVE' OR password_hash IS NOT NULL) NOT VALID;

CREATE UNIQUE INDEX users_email_unique
ON users ((lower(email)))
WHERE email IS NOT NULL;

CREATE TABLE businesses (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(200) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'ACTIVE',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT businesses_name_not_empty CHECK (btrim(name) <> ''),
    CONSTRAINT businesses_status_check CHECK (status IN ('ACTIVE', 'SUSPENDED'))
);

CREATE TABLE business_memberships (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    business_id UUID NOT NULL REFERENCES businesses(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role VARCHAR(20) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT business_memberships_role_check CHECK (role IN ('OWNER', 'ADMIN', 'STAFF')),
    CONSTRAINT business_memberships_unique UNIQUE (business_id, user_id)
);

CREATE INDEX idx_business_memberships_user_id ON business_memberships(user_id);

CREATE TABLE audit_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    business_id UUID REFERENCES businesses(id) ON DELETE SET NULL,
    event_type VARCHAR(100) NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT audit_events_event_type_not_empty CHECK (btrim(event_type) <> ''),
    CONSTRAINT audit_events_metadata_object CHECK (jsonb_typeof(metadata) = 'object')
);

CREATE INDEX idx_audit_events_actor_user_id ON audit_events(actor_user_id);
CREATE INDEX idx_audit_events_business_id ON audit_events(business_id);
CREATE INDEX idx_audit_events_created_at ON audit_events(created_at);
