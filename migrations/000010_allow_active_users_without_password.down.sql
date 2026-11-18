ALTER TABLE users ADD CONSTRAINT users_active_password_hash_check CHECK (status <> 'ACTIVE' OR password_hash IS NOT NULL) NOT VALID;
