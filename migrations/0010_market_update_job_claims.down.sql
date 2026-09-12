DO $$
BEGIN
    IF to_regclass('cc_market_update_job_claim_journal') IS NOT NULL
       AND EXISTS (SELECT 1 FROM cc_market_update_job_claim_journal LIMIT 1) THEN
        RAISE EXCEPTION 'cannot downgrade while Market update lifecycle state exists';
    END IF;
    IF to_regclass('cc_market_update_jobs') IS NOT NULL
       AND EXISTS (SELECT 1 FROM cc_market_update_jobs LIMIT 1) THEN
        RAISE EXCEPTION 'cannot downgrade while Market update lifecycle state exists';
    END IF;

    IF to_regclass('cc_market_update_job_claim_journal') IS NOT NULL THEN
        EXECUTE 'DROP TRIGGER IF EXISTS cc_market_update_job_claim_journal_pair '
            || 'ON cc_market_update_job_claim_journal';
    END IF;
    IF to_regclass('cc_market_update_jobs') IS NOT NULL THEN
        EXECUTE 'DROP TRIGGER IF EXISTS cc_market_update_jobs_claim_pair ON cc_market_update_jobs';
    END IF;
END;
$$;

DROP FUNCTION IF EXISTS cc_market_validate_update_job_claim_pair();
DROP TABLE IF EXISTS cc_market_update_job_claim_journal;
DROP TABLE IF EXISTS cc_market_update_jobs;
