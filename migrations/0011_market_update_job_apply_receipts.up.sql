ALTER TABLE cc_market_update_jobs
    DROP CONSTRAINT IF EXISTS cc_market_update_jobs_state_check;
ALTER TABLE cc_market_update_jobs
    ADD CONSTRAINT cc_market_update_jobs_state_check
    CHECK (state IN ('PREPARED', 'RUNNING', 'VERIFYING'));

ALTER TABLE cc_market_update_jobs
    DROP CONSTRAINT IF EXISTS cc_market_update_jobs_state_shape;
ALTER TABLE cc_market_update_jobs
    ADD CONSTRAINT cc_market_update_jobs_state_shape CHECK (
        (
            state = 'PREPARED'
            AND state_version = 1
            AND journal_sequence = 0
            AND claim_id IS NULL
            AND claimed_by IS NULL
            AND claimed_from_state_version IS NULL
        )
        OR
        (
            state = 'RUNNING'
            AND state_version = 2
            AND journal_sequence = 1
            AND claim_id IS NOT NULL
            AND btrim(claim_id) <> ''
            AND claimed_by IS NOT NULL
            AND btrim(claimed_by) <> ''
            AND claimed_from_state_version = 1
        )
        OR
        (
            state = 'VERIFYING'
            AND state_version = 3
            AND journal_sequence = 2
            AND claim_id IS NOT NULL
            AND btrim(claim_id) <> ''
            AND claimed_by IS NOT NULL
            AND btrim(claimed_by) <> ''
            AND claimed_from_state_version = 1
        )
    );

CREATE TABLE IF NOT EXISTS cc_market_update_job_apply_journal (
    receipt_id text PRIMARY KEY CHECK (btrim(receipt_id) <> ''),
    record_id text NOT NULL REFERENCES cc_market_update_jobs(record_id) ON DELETE RESTRICT,
    job_id text NOT NULL CHECK (btrim(job_id) <> ''),
    admission_id text NOT NULL CHECK (btrim(admission_id) <> ''),
    claim_id text NOT NULL CHECK (btrim(claim_id) <> ''),
    worker_id text NOT NULL CHECK (btrim(worker_id) <> ''),
    bundle_id text NOT NULL CHECK (btrim(bundle_id) <> ''),
    bundle_sha256 text NOT NULL CHECK (bundle_sha256 ~ '^[0-9a-f]{64}$'),
    bundle_size_bytes bigint NOT NULL CHECK (bundle_size_bytes > 0),
    current_version text NOT NULL CHECK (btrim(current_version) <> ''),
    target_version text NOT NULL CHECK (btrim(target_version) <> ''),
    generation bigint NOT NULL CHECK (generation > 0),
    migration_count integer NOT NULL CHECK (migration_count >= 0),
    rollback_mode text NOT NULL CHECK (btrim(rollback_mode) <> ''),
    require_pre_migration_snapshot boolean NOT NULL,
    pre_migration_snapshot_id text,
    from_state text NOT NULL CHECK (from_state = 'RUNNING'),
    to_state text NOT NULL CHECK (to_state = 'VERIFYING'),
    from_state_version bigint NOT NULL CHECK (from_state_version = 2),
    to_state_version bigint NOT NULL CHECK (to_state_version = 3),
    journal_sequence bigint NOT NULL CHECK (journal_sequence = 2),
    preserve_user_data boolean NOT NULL DEFAULT true CHECK (preserve_user_data),
    preserve_secret_material boolean NOT NULL DEFAULT true CHECK (preserve_secret_material),
    verification_required boolean NOT NULL DEFAULT true CHECK (verification_required),
    rollback_required_on_failure boolean NOT NULL DEFAULT true CHECK (rollback_required_on_failure),
    execution_authorized boolean NOT NULL DEFAULT false CHECK (NOT execution_authorized),
    production_mutation_allowed boolean NOT NULL DEFAULT false CHECK (NOT production_mutation_allowed),
    CONSTRAINT cc_market_update_job_apply_snapshot_shape CHECK (
        (require_pre_migration_snapshot AND pre_migration_snapshot_id IS NOT NULL AND btrim(pre_migration_snapshot_id) <> '')
        OR
        (NOT require_pre_migration_snapshot AND pre_migration_snapshot_id IS NULL)
    ),
    CONSTRAINT cc_market_update_job_apply_record_sequence UNIQUE (record_id, journal_sequence),
    CONSTRAINT cc_market_update_job_apply_record_receipt UNIQUE (record_id, receipt_id)
);

CREATE OR REPLACE FUNCTION cc_market_validate_update_job_claim_pair()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    target_record_id text;
    persisted_state text;
    persisted_job_id text;
    persisted_admission_id text;
    persisted_claim_id text;
    persisted_worker_id text;
    persisted_state_version bigint;
    persisted_journal_sequence bigint;
    persisted_claimed_from bigint;
    claim_count bigint;
    apply_count bigint;
BEGIN
    IF TG_OP = 'DELETE' THEN
        target_record_id := OLD.record_id;
    ELSE
        target_record_id := NEW.record_id;
    END IF;

    SELECT state, job_id, admission_id, claim_id, claimed_by,
           state_version, journal_sequence, claimed_from_state_version
    INTO persisted_state, persisted_job_id, persisted_admission_id, persisted_claim_id,
         persisted_worker_id, persisted_state_version, persisted_journal_sequence, persisted_claimed_from
    FROM cc_market_update_jobs
    WHERE record_id = target_record_id;

    IF NOT FOUND THEN
        RETURN NULL;
    END IF;

    SELECT count(*) INTO claim_count
    FROM cc_market_update_job_claim_journal
    WHERE record_id = target_record_id;

    SELECT count(*) INTO apply_count
    FROM cc_market_update_job_apply_journal
    WHERE record_id = target_record_id;

    IF persisted_state = 'PREPARED' THEN
        IF claim_count <> 0 OR apply_count <> 0 THEN
            RAISE EXCEPTION 'PREPARED Market update job must not have lifecycle journal evidence';
        END IF;
        RETURN NULL;
    END IF;

    IF claim_count <> 1 OR NOT EXISTS (
        SELECT 1
        FROM cc_market_update_job_claim_journal j
        WHERE j.record_id = target_record_id
          AND j.sequence = 1
          AND j.job_id = persisted_job_id
          AND j.admission_id = persisted_admission_id
          AND j.claim_id = persisted_claim_id
          AND j.worker_id = persisted_worker_id
          AND j.from_state = 'PREPARED'
          AND j.to_state = 'RUNNING'
          AND j.from_state_version = persisted_claimed_from
          AND j.to_state_version = 2
          AND NOT j.execution_authorized
    ) THEN
        RAISE EXCEPTION 'Market update job claim record and journal evidence do not match';
    END IF;

    IF persisted_state = 'RUNNING' THEN
        IF persisted_state_version <> 2 OR persisted_journal_sequence <> 1 OR apply_count <> 0 THEN
            RAISE EXCEPTION 'RUNNING Market update job must not have apply receipt evidence';
        END IF;
        RETURN NULL;
    END IF;

    IF persisted_state <> 'VERIFYING' OR apply_count <> 1 THEN
        RAISE EXCEPTION 'VERIFYING Market update job requires exactly one apply receipt';
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM cc_market_update_job_apply_journal a
        WHERE a.record_id = target_record_id
          AND a.job_id = persisted_job_id
          AND a.admission_id = persisted_admission_id
          AND a.claim_id = persisted_claim_id
          AND a.worker_id = persisted_worker_id
          AND a.from_state = 'RUNNING'
          AND a.to_state = 'VERIFYING'
          AND a.from_state_version = 2
          AND a.to_state_version = persisted_state_version
          AND a.journal_sequence = persisted_journal_sequence
          AND a.preserve_user_data
          AND a.preserve_secret_material
          AND a.verification_required
          AND a.rollback_required_on_failure
          AND NOT a.execution_authorized
          AND NOT a.production_mutation_allowed
    ) THEN
        RAISE EXCEPTION 'Market update VERIFYING record and apply receipt evidence do not match';
    END IF;

    RETURN NULL;
END;
$$;

DROP TRIGGER IF EXISTS cc_market_update_job_apply_journal_pair ON cc_market_update_job_apply_journal;
CREATE CONSTRAINT TRIGGER cc_market_update_job_apply_journal_pair
AFTER INSERT OR UPDATE OR DELETE ON cc_market_update_job_apply_journal
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW
EXECUTE FUNCTION cc_market_validate_update_job_claim_pair();
