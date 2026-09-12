DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM cc_market_update_job_rollback_completion_journal LIMIT 1) THEN
        RAISE EXCEPTION 'cannot downgrade while Market update rollback completion evidence exists';
    END IF;
END;
$$;

DROP TRIGGER IF EXISTS cc_market_update_job_rollback_completion_pair
    ON cc_market_update_job_rollback_completion_journal;
DROP FUNCTION IF EXISTS cc_market_validate_update_job_rollback_completion();
DROP TABLE IF EXISTS cc_market_update_job_rollback_completion_journal;
