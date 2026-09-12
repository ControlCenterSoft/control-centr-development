DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM cc_market_update_job_verification_journal LIMIT 1)
       OR EXISTS (
           SELECT 1
           FROM cc_market_update_jobs
           WHERE state IN ('SUCCEEDED', 'ROLLING_BACK')
           LIMIT 1
       ) THEN
        RAISE EXCEPTION 'cannot downgrade while Market update verification receipt state exists';
    END IF;
END;
$$;

DROP TRIGGER IF EXISTS cc_market_update_job_state_pair ON cc_market_update_jobs;
DROP TRIGGER IF EXISTS cc_market_update_job_verification_journal_pair
    ON cc_market_update_job_verification_journal;
DROP TABLE IF EXISTS cc_market_update_job_verification_journal;

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

ALTER TABLE cc_market_update_jobs
    DROP CONSTRAINT IF EXISTS cc_market_update_jobs_state_check;
ALTER TABLE cc_market_update_jobs
    ADD CONSTRAINT cc_market_update_jobs_state_check
    CHECK (state IN ('PREPARED', 'RUNNING', 'VERIFYING'));

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
