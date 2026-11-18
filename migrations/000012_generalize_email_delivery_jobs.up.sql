ALTER TABLE email_delivery_jobs
    DROP CONSTRAINT email_delivery_jobs_type_check,
    DROP CONSTRAINT email_delivery_jobs_verification_unique;

ALTER TABLE email_delivery_jobs
    ADD CONSTRAINT email_delivery_jobs_type_check
        CHECK (job_type IN ('EMAIL_VERIFICATION', 'WELCOME')),
    ADD CONSTRAINT email_delivery_jobs_verification_type_unique
        UNIQUE (verification_id, job_type);

