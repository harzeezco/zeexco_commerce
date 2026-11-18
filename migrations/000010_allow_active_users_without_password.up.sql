-- ACTIVE WhatsApp-first users authenticate through the identity linked in the
-- same registration transaction. A CHECK cannot reference that table, so the
-- application transaction enforces "password or identity".
ALTER TABLE users DROP CONSTRAINT users_active_password_hash_check;
