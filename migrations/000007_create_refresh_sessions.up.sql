CREATE TABLE refresh_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash BYTEA NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    last_used_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    replaced_by_session_id UUID REFERENCES refresh_sessions(id) ON DELETE SET NULL,

    CONSTRAINT refresh_sessions_token_hash_unique UNIQUE (token_hash),
    CONSTRAINT refresh_sessions_token_hash_length CHECK (octet_length(token_hash) = 32),
    CONSTRAINT refresh_sessions_expiration_check CHECK (expires_at > created_at),
    CONSTRAINT refresh_sessions_last_used_check CHECK (last_used_at IS NULL OR last_used_at >= created_at),
    CONSTRAINT refresh_sessions_revoked_check CHECK (revoked_at IS NULL OR revoked_at >= created_at)
);

CREATE INDEX idx_refresh_sessions_user_id ON refresh_sessions(user_id);
CREATE INDEX idx_refresh_sessions_active_expiration
ON refresh_sessions(expires_at)
WHERE revoked_at IS NULL;
