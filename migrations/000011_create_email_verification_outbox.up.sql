CREATE TABLE email_verifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    verified_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT email_verifications_expiration_check CHECK (expires_at > created_at),
    CONSTRAINT email_verifications_verified_check CHECK (verified_at IS NULL OR verified_at >= created_at)
);

CREATE UNIQUE INDEX email_verifications_one_pending_user
ON email_verifications(user_id)
WHERE verified_at IS NULL;

CREATE TABLE email_delivery_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    verification_id UUID NOT NULL REFERENCES email_verifications(id) ON DELETE CASCADE,
    job_type VARCHAR(50) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'PENDING',
    attempt_count INTEGER NOT NULL DEFAULT 0,
    available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processing_started_at TIMESTAMPTZ,
    sent_at TIMESTAMPTZ,
    provider_message_id VARCHAR(255),
    last_error VARCHAR(512),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT email_delivery_jobs_type_check CHECK (job_type = 'EMAIL_VERIFICATION'),
    CONSTRAINT email_delivery_jobs_status_check CHECK (status IN ('PENDING', 'PROCESSING', 'FAILED', 'SENT')),
    CONSTRAINT email_delivery_jobs_attempt_check CHECK (attempt_count >= 0),
    CONSTRAINT email_delivery_jobs_verification_unique UNIQUE (verification_id),
    CONSTRAINT email_delivery_jobs_lifecycle_check CHECK (
        (status IN ('PENDING', 'FAILED') AND processing_started_at IS NULL AND sent_at IS NULL)
        OR (status = 'PROCESSING' AND processing_started_at IS NOT NULL AND sent_at IS NULL)
        OR (status = 'SENT' AND processing_started_at IS NULL AND sent_at IS NOT NULL)
    )
);

CREATE INDEX email_delivery_jobs_claimable
ON email_delivery_jobs(status, available_at)
WHERE status IN ('PENDING', 'FAILED', 'PROCESSING');
