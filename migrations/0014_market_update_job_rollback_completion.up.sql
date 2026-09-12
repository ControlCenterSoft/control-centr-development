CREATE TABLE IF NOT EXISTS cc_market_update_job_rollback_completion_journal (
    receipt_id text PRIMARY KEY CHECK (btrim(receipt_id) <> ''),
    attempt_id text NOT NULL UNIQUE CHECK (btrim(attempt_id) <> ''),
    claim_id text NOT NULL UNIQUE
        REFERENCES cc_market_update_job_rollback_claims(claim_id) ON DELETE RESTRICT,
    rollback_admission_id text NOT NULL CHECK (btrim(rollback_admission_id) <> ''),
    record_id text NOT NULL UNIQUE REFERENCES cc_market_update_jobs(record_id) ON DELETE RESTRICT,
    job_id text NOT NULL CHECK (btrim(job_id) <> ''),
    source_admission_id text NOT NULL CHECK (btrim(source_admission_id) <> ''),
    verification_receipt_id text NOT NULL
        REFERENCES cc_market_update_job_verification_journal(receipt_id) ON DELETE RESTRICT,
    worker_id text NOT NULL CHECK (btrim(worker_id) <> ''),
    module_id text NOT NULL CHECK (btrim(module_id) <> ''),
    restore_version text NOT NULL CHECK (btrim(restore_version) <> ''),
    failed_target_version text NOT NULL CHECK (btrim(failed_target_version) <> ''),
    generation bigint NOT NULL CHECK (generation > 0),
    rollback_mode text NOT NULL CHECK (btrim(rollback_mode) <> ''),
    pre_migration_snapshot_id text NOT NULL DEFAULT '',
    outcome text NOT NULL CHECK (outcome IN ('SUCCEEDED', 'FAILED')),
    from_state text NOT NULL CHECK (from_state = 'ROLLING_BACK'),
    to_state text NOT NULL CHECK (to_state IN ('ROLLED_BACK', 'FAILED')),
    from_state_version bigint NOT NULL CHECK (from_state_version = 4),
    to_state_version bigint NOT NULL CHECK (to_state_version = from_state_version + 1),
    journal_sequence bigint NOT NULL CHECK (journal_sequence = 5),
    attempt_consumed boolean NOT NULL DEFAULT true CHECK (attempt_consumed),
    atomic_persistence_required boolean NOT NULL DEFAULT true CHECK (atomic_persistence_required),
    preserve_user_data boolean NOT NULL DEFAULT true CHECK (preserve_user_data),
    preserve_secret_material boolean NOT NULL DEFAULT true CHECK (preserve_secret_material),
    terminal boolean NOT NULL DEFAULT true CHECK (terminal),
    recovery_required boolean NOT NULL,
    further_attempt_authorized boolean NOT NULL DEFAULT false CHECK (NOT further_attempt_authorized),
    automatic_retry_authorized boolean NOT NULL DEFAULT false CHECK (NOT automatic_retry_authorized),
    execution_authorized boolean NOT NULL DEFAULT false CHECK (NOT execution_authorized),
    generic_command_authorized boolean NOT NULL DEFAULT false CHECK (NOT generic_command_authorized),
    production_mutation_allowed boolean NOT NULL DEFAULT false CHECK (NOT production_mutation_allowed),
    source_state jsonb NOT NULL CHECK (jsonb_typeof(source_state) = 'object'),
    attempt jsonb NOT NULL CHECK (jsonb_typeof(attempt) = 'object'),
    CONSTRAINT cc_market_update_job_rollback_completion_outcome CHECK (
        (outcome = 'SUCCEEDED' AND to_state = 'ROLLED_BACK' AND NOT recovery_required)
        OR (outcome = 'FAILED' AND to_state = 'FAILED' AND recovery_required)
    )
);

CREATE OR REPLACE FUNCTION cc_market_validate_update_job_rollback_completion()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF TG_OP = 'UPDATE' AND NEW IS DISTINCT FROM OLD THEN
        RAISE EXCEPTION 'Market update rollback completion evidence is immutable';
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM cc_market_update_jobs j
        JOIN cc_market_update_job_rollback_claims c
          ON c.record_id = j.record_id
         AND c.claim_id = NEW.claim_id
        JOIN cc_market_update_job_verification_journal v
          ON v.record_id = j.record_id
         AND v.receipt_id = NEW.verification_receipt_id
        WHERE j.record_id = NEW.record_id
          AND j.job_id = NEW.job_id
          AND j.admission_id = NEW.source_admission_id
          AND j.state = NEW.to_state
          AND j.state_version = NEW.to_state_version
          AND j.journal_sequence = NEW.journal_sequence
          AND j.preserve_user_data
          AND j.preserve_secret_material
          AND NOT j.execution_authorized
          AND c.rollback_admission_id = NEW.rollback_admission_id
          AND c.job_id = NEW.job_id
          AND c.source_admission_id = NEW.source_admission_id
          AND c.verification_receipt_id = NEW.verification_receipt_id
          AND c.worker_id = NEW.worker_id
          AND c.expected_state_version = NEW.from_state_version
          AND c.claim_sequence + 1 = NEW.journal_sequence
          AND c.single_use
          AND c.atomic_persistence_required
          AND c.fresh_revalidation_required
          AND c.preserve_user_data
          AND c.preserve_secret_material
          AND NOT c.execution_authorized
          AND NOT c.production_mutation_allowed
          AND v.outcome = 'FAILED'
          AND v.to_state = 'ROLLING_BACK'
          AND v.to_state_version = NEW.from_state_version
          AND v.preserve_user_data
          AND v.preserve_secret_material
          AND v.rollback_required_now
          AND NOT v.terminal
          AND NOT v.execution_authorized
          AND NOT v.production_mutation_allowed
    ) THEN
        RAISE EXCEPTION 'Market update rollback completion does not match durable claim/job evidence';
    END IF;

    IF NEW.attempt->>'attempt_id' IS DISTINCT FROM NEW.attempt_id
       OR NEW.attempt->>'claim_id' IS DISTINCT FROM NEW.claim_id
       OR NEW.attempt->>'rollback_admission_id' IS DISTINCT FROM NEW.rollback_admission_id
       OR NEW.attempt->>'record_id' IS DISTINCT FROM NEW.record_id
       OR NEW.attempt->>'job_id' IS DISTINCT FROM NEW.job_id
       OR NEW.attempt->>'source_admission_id' IS DISTINCT FROM NEW.source_admission_id
       OR NEW.attempt->>'verification_receipt_id' IS DISTINCT FROM NEW.verification_receipt_id
       OR NEW.attempt->>'worker_id' IS DISTINCT FROM NEW.worker_id
       OR NEW.attempt->>'module_id' IS DISTINCT FROM NEW.module_id
       OR NEW.attempt->>'restore_version' IS DISTINCT FROM NEW.restore_version
       OR NEW.attempt->>'failed_target_version' IS DISTINCT FROM NEW.failed_target_version
    THEN
        RAISE EXCEPTION 'Market update rollback completion attempt JSON lineage mismatch';
    END IF;

    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS cc_market_update_job_rollback_completion_pair
    ON cc_market_update_job_rollback_completion_journal;
CREATE CONSTRAINT TRIGGER cc_market_update_job_rollback_completion_pair
AFTER INSERT OR UPDATE ON cc_market_update_job_rollback_completion_journal
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW
EXECUTE FUNCTION cc_market_validate_update_job_rollback_completion();
