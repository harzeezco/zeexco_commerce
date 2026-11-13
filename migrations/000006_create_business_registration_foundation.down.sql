DROP TABLE IF EXISTS audit_events;
DROP TABLE IF EXISTS business_memberships;
DROP TABLE IF EXISTS businesses;
DROP INDEX IF EXISTS users_email_unique;

ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_active_password_hash_check,
    DROP CONSTRAINT IF EXISTS users_password_hash_not_empty,
    DROP COLUMN IF EXISTS password_hash;
