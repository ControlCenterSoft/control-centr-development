CREATE TABLE IF NOT EXISTS cc_market_update_job_rollback_claims (
    claim_id text PRIMARY KEY CHECK (btrim(claim_id) <> ''),
    rollback_admission_id text NOT NULL UNIQUE CHECK (btrim(rollback_admission_id) <> ''),
    record_id text NOT NULL UNIQUE REFERENCES cc_market_update_jobs(record_id) ON DELETE RESTRICT,
    job_id text NOT NULL CHECK (btrim(job_id) <> ''),
    source_admission_id text NOT NULL CHECK (btrim(source_admission_id) <> ''),
    verification_receipt_id text NOT NULL UNIQUE
        REFERENCES cc_market_update_job_verification_journal(receipt_id) ON DELETE RESTRICT,
    worker_id text NOT NULL CHECK (
        char_length(worker_id) BETWEEN 1 AND 128
        AND btrim(worker_id) = worker_id
        AND (
            worker_id ~ '^[A-Za-z0-9]$'
            OR worker_id ~ '^[A-Za-z0-9][A-Za-z0-9._:/-]*[A-Za-z0-9]$'
        )
    ),
    expected_state_version bigint NOT NULL CHECK (expected_state_version = 4),
    expected_journal_sequence bigint NOT NULL CHECK (expected_journal_sequence = 3),
    claim_sequence bigint NOT NULL CHECK (claim_sequence = expected_journal_sequence + 1),
    single_use boolean NOT NULL DEFAULT true CHECK (single_use),
    atomic_persistence_required boolean NOT NULL DEFAULT true CHECK (atomic_persistence_required),
    fresh_revalidation_required boolean NOT NULL DEFAULT true CHECK (fresh_revalidation_required),
    preserve_user_data boolean NOT NULL DEFAULT true CHECK (preserve_user_data),
    preserve_secret_material boolean NOT NULL DEFAULT true CHECK (preserve_secret_material),
    execution_authorized boolean NOT NULL DEFAULT false CHECK (NOT execution_authorized),
    production_mutation_allowed boolean NOT NULL DEFAULT false CHECK (NOT production_mutation_allowed),
    CONSTRAINT cc_market_update_job_rollback_claim_sequence
        UNIQUE (record_id, claim_sequence)
);

CREATE OR REPLACE FUNCTION cc_market_validate_update_job_rollback_claim()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF TG_OP = 'UPDATE' AND NEW IS DISTINCT FROM OLD THEN
        RAISE EXCEPTION 'Market update rollback claim evidence is immutable';
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM cc_market_update_jobs j
        JOIN cc_market_update_job_verification_journal v
          ON v.record_id = j.record_id
         AND v.receipt_id = NEW.verification_receipt_id
        WHERE j.record_id = NEW.record_id
          AND j.job_id = NEW.job_id
          AND j.admission_id = NEW.source_admission_id
          AND j.state = 'ROLLING_BACK'
          AND j.state_version = NEW.expected_state_version
          AND j.journal_sequence = NEW.expected_journal_sequence
          AND j.preserve_user_data
          AND j.preserve_secret_material
          AND NOT j.execution_authorized
          AND v.job_id = j.job_id
          AND v.admission_id = j.admission_id
          AND v.outcome = 'FAILED'
          AND v.from_state = 'VERIFYING'
          AND v.to_state = 'ROLLING_BACK'
          AND v.from_state_version = 3
          AND v.to_state_version = NEW.expected_state_version
          AND v.journal_sequence = NEW.expected_journal_sequence
          AND v.preserve_user_data
          AND v.preserve_secret_material
          AND v.rollback_required_now
          AND NOT v.terminal
          AND NOT v.execution_authorized
          AND NOT v.production_mutation_allowed
    ) THEN
        RAISE EXCEPTION 'Market update rollback claim does not match durable failed-verification evidence';
    END IF;

    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS cc_market_update_job_rollback_claim_pair
    ON cc_market_update_job_rollback_claims;
CREATE CONSTRAINT TRIGGER cc_market_update_job_rollback_claim_pair
AFTER INSERT OR UPDATE ON cc_market_update_job_rollback_claims
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW
EXECUTE FUNCTION cc_market_validate_update_job_rollback_claim();
