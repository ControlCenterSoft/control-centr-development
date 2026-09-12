DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM cc_market_update_job_rollback_claims LIMIT 1) THEN
        RAISE EXCEPTION 'cannot downgrade while Market update rollback claim evidence exists';
    END IF;
END;
$$;

DROP TRIGGER IF EXISTS cc_market_update_job_rollback_claim_pair
    ON cc_market_update_job_rollback_claims;
DROP FUNCTION IF EXISTS cc_market_validate_update_job_rollback_claim();
DROP TABLE IF EXISTS cc_market_update_job_rollback_claims;
