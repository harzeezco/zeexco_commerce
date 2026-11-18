ALTER TABLE email_delivery_jobs
    DROP CONSTRAINT email_delivery_jobs_verification_type_unique;

CREATE UNIQUE INDEX email_delivery_jobs_one_welcome_per_verification
    ON email_delivery_jobs (verification_id)
    WHERE job_type = 'WELCOME';
