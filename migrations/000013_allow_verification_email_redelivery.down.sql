DROP INDEX email_delivery_jobs_one_welcome_per_verification;

DELETE FROM email_delivery_jobs newer
USING email_delivery_jobs older
WHERE newer.verification_id = older.verification_id
  AND newer.job_type = 'EMAIL_VERIFICATION'
  AND older.job_type = 'EMAIL_VERIFICATION'
  AND (newer.created_at, newer.id) > (older.created_at, older.id);

ALTER TABLE email_delivery_jobs
    ADD CONSTRAINT email_delivery_jobs_verification_type_unique
        UNIQUE (verification_id, job_type);
