package migrations

import (
	"os"
	"strings"
	"testing"
)

func TestMarketUpdateJobClaimMigrationSealsAtomicPair(t *testing.T) {
	up := readMarketClaimMigration(t, "0010_market_update_job_claims.up.sql")
	for _, required := range []string{
		"CREATE TABLE IF NOT EXISTS cc_market_update_jobs",
		"CREATE TABLE IF NOT EXISTS cc_market_update_job_claim_journal",
		"CONSTRAINT cc_market_update_jobs_state_shape CHECK",
		"CONSTRAINT cc_market_update_job_claim_journal_sequence UNIQUE (record_id, sequence)",
		"CONSTRAINT cc_market_update_job_claim_journal_claim UNIQUE (record_id, claim_id)",
		"CHECK (preserve_user_data)",
		"CHECK (preserve_secret_material)",
		"CHECK (NOT execution_authorized)",
		"CREATE CONSTRAINT TRIGGER cc_market_update_jobs_claim_pair",
		"CREATE CONSTRAINT TRIGGER cc_market_update_job_claim_journal_pair",
		"DEFERRABLE INITIALLY DEFERRED",
		"RUNNING Market update job requires exactly one claim journal entry",
		"Market update job claim record and journal evidence do not match",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("0010 up migration lacks %q", required)
		}
	}
}

func TestMarketUpdateJobClaimMigrationIsScopedAndFailClosed(t *testing.T) {
	up := strings.ToUpper(readMarketClaimMigration(t, "0010_market_update_job_claims.up.sql"))
	for _, forbidden := range []string{
		"ALTER TABLE CC_LOCAL_USERS",
		"ALTER TABLE CC_AUTH_SESSIONS",
		"ALTER TABLE CC_CORE_OBJECTS",
		"DROP TABLE CC_LOCAL_USERS",
		"DROP TABLE CC_AUTH_SESSIONS",
		"DROP TABLE CC_CORE_OBJECTS",
		"CASCADE",
	} {
		if strings.Contains(up, forbidden) {
			t.Fatalf("0010 Market migration widens scope with %q", forbidden)
		}
	}

	down := strings.ToUpper(readMarketClaimMigration(t, "0010_market_update_job_claims.down.sql"))
	for _, required := range []string{
		"EXISTS (SELECT 1 FROM CC_MARKET_UPDATE_JOB_CLAIM_JOURNAL LIMIT 1)",
		"EXISTS (SELECT 1 FROM CC_MARKET_UPDATE_JOBS LIMIT 1)",
		"RAISE EXCEPTION 'CANNOT DOWNGRADE WHILE MARKET UPDATE LIFECYCLE STATE EXISTS'",
		"DROP TABLE IF EXISTS CC_MARKET_UPDATE_JOB_CLAIM_JOURNAL",
		"DROP TABLE IF EXISTS CC_MARKET_UPDATE_JOBS",
	} {
		if !strings.Contains(down, required) {
			t.Fatalf("0010 down migration lacks %q", required)
		}
	}
	if strings.Contains(down, "CASCADE") {
		t.Fatal("0010 down migration must not cascade")
	}
}

func TestMarketUpdateJobClaimMigrationRejectsTornDurableShapes(t *testing.T) {
	up := readMarketClaimMigration(t, "0010_market_update_job_claims.up.sql")
	for _, required := range []string{
		"state = 'PREPARED'",
		"journal_count <> 0",
		"state = 'RUNNING'",
		"journal_count <> 1",
		"j.sequence = persisted_journal_sequence",
		"j.claim_id = persisted_claim_id",
		"j.worker_id = persisted_worker_id",
		"j.from_state_version = persisted_claimed_from",
		"j.to_state_version = persisted_state_version",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("0010 does not fail closed for torn claim evidence: missing %q", required)
		}
	}
}

func readMarketClaimMigration(t *testing.T, name string) string {
	t.Helper()
	content, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
