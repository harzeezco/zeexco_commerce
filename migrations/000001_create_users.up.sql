CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    first_name VARCHAR(100),
    last_name VARCHAR(100),

    email VARCHAR(255),

    status VARCHAR(20) NOT NULL DEFAULT 'PENDING',

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT users_status_check
        CHECK (status IN ('PENDING', 'ACTIVE', 'SUSPENDED')),

    CONSTRAINT users_active_profile_check
        CHECK (
            status <> 'ACTIVE'
            OR (
                first_name IS NOT NULL
                AND btrim(first_name) <> ''
                AND last_name IS NOT NULL
                AND btrim(last_name) <> ''
            )
        ),

    CONSTRAINT users_email_not_empty
        CHECK (email IS NULL OR btrim(email) <> '')
);