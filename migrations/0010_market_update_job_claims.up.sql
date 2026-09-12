CREATE TABLE IF NOT EXISTS cc_market_update_jobs (
    record_id text PRIMARY KEY CHECK (btrim(record_id) <> ''),
    job_id text NOT NULL CHECK (btrim(job_id) <> ''),
    admission_id text NOT NULL CHECK (btrim(admission_id) <> ''),
    state text NOT NULL CHECK (state IN ('PREPARED', 'RUNNING')),
    state_version bigint NOT NULL CHECK (state_version > 0),
    journal_sequence bigint NOT NULL CHECK (journal_sequence >= 0),
    claim_id text,
    claimed_by text,
    claimed_from_state_version bigint,
    preserve_user_data boolean NOT NULL DEFAULT true CHECK (preserve_user_data),
    preserve_secret_material boolean NOT NULL DEFAULT true CHECK (preserve_secret_material),
    execution_authorized boolean NOT NULL DEFAULT false CHECK (NOT execution_authorized),
    CONSTRAINT cc_market_update_jobs_admission_identity UNIQUE (job_id, admission_id),
    CONSTRAINT cc_market_update_jobs_state_shape CHECK (
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
    )
);

CREATE TABLE IF NOT EXISTS cc_market_update_job_claim_journal (
    journal_id text PRIMARY KEY CHECK (btrim(journal_id) <> ''),
    record_id text NOT NULL REFERENCES cc_market_update_jobs(record_id) ON DELETE RESTRICT,
    sequence bigint NOT NULL CHECK (sequence = 1),
    job_id text NOT NULL CHECK (btrim(job_id) <> ''),
    admission_id text NOT NULL CHECK (btrim(admission_id) <> ''),
    claim_id text NOT NULL CHECK (btrim(claim_id) <> ''),
    worker_id text NOT NULL CHECK (btrim(worker_id) <> ''),
    from_state text NOT NULL CHECK (from_state = 'PREPARED'),
    to_state text NOT NULL CHECK (to_state = 'RUNNING'),
    from_state_version bigint NOT NULL CHECK (from_state_version = 1),
    to_state_version bigint NOT NULL CHECK (to_state_version = 2),
    execution_authorized boolean NOT NULL DEFAULT false CHECK (NOT execution_authorized),
    CONSTRAINT cc_market_update_job_claim_journal_sequence UNIQUE (record_id, sequence),
    CONSTRAINT cc_market_update_job_claim_journal_claim UNIQUE (record_id, claim_id)
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
    journal_count bigint;
BEGIN
    IF TG_OP = 'DELETE' THEN
        target_record_id := OLD.record_id;
    ELSE
        target_record_id := NEW.record_id;
    END IF;

    SELECT
        state,
        job_id,
        admission_id,
        claim_id,
        claimed_by,
        state_version,
        journal_sequence,
        claimed_from_state_version
    INTO
        persisted_state,
        persisted_job_id,
        persisted_admission_id,
        persisted_claim_id,
        persisted_worker_id,
        persisted_state_version,
        persisted_journal_sequence,
        persisted_claimed_from
    FROM cc_market_update_jobs
    WHERE record_id = target_record_id;

    IF NOT FOUND THEN
        RETURN NULL;
    END IF;

    SELECT count(*) INTO journal_count
    FROM cc_market_update_job_claim_journal
    WHERE record_id = target_record_id;

    IF persisted_state = 'PREPARED' THEN
        IF journal_count <> 0 THEN
            RAISE EXCEPTION 'PREPARED Market update job must not have claim journal evidence';
        END IF;
        RETURN NULL;
    END IF;

    IF persisted_state <> 'RUNNING' OR journal_count <> 1 THEN
        RAISE EXCEPTION 'RUNNING Market update job requires exactly one claim journal entry';
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM cc_market_update_job_claim_journal j
        WHERE j.record_id = target_record_id
          AND j.sequence = persisted_journal_sequence
          AND j.job_id = persisted_job_id
          AND j.admission_id = persisted_admission_id
          AND j.claim_id = persisted_claim_id
          AND j.worker_id = persisted_worker_id
          AND j.from_state = 'PREPARED'
          AND j.to_state = 'RUNNING'
          AND j.from_state_version = persisted_claimed_from
          AND j.to_state_version = persisted_state_version
          AND NOT j.execution_authorized
    ) THEN
        RAISE EXCEPTION 'Market update job claim record and journal evidence do not match';
    END IF;

    RETURN NULL;
END;
$$;

DROP TRIGGER IF EXISTS cc_market_update_jobs_claim_pair ON cc_market_update_jobs;
CREATE CONSTRAINT TRIGGER cc_market_update_jobs_claim_pair
AFTER INSERT OR UPDATE ON cc_market_update_jobs
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW
EXECUTE FUNCTION cc_market_validate_update_job_claim_pair();

DROP TRIGGER IF EXISTS cc_market_update_job_claim_journal_pair ON cc_market_update_job_claim_journal;
CREATE CONSTRAINT TRIGGER cc_market_update_job_claim_journal_pair
AFTER INSERT OR UPDATE OR DELETE ON cc_market_update_job_claim_journal
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW
EXECUTE FUNCTION cc_market_validate_update_job_claim_pair();
