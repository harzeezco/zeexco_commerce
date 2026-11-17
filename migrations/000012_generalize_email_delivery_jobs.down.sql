DELETE FROM email_delivery_jobs WHERE job_type = 'WELCOME';

ALTER TABLE email_delivery_jobs
    DROP CONSTRAINT email_delivery_jobs_verification_type_unique,
    DROP CONSTRAINT email_delivery_jobs_type_check;

ALTER TABLE email_delivery_jobs
    ADD CONSTRAINT email_delivery_jobs_type_check
        CHECK (job_type = 'EMAIL_VERIFICATION'),
    ADD CONSTRAINT email_delivery_jobs_verification_unique
        UNIQUE (verification_id);

