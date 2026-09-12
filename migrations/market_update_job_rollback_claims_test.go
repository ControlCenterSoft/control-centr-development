package migrations

import (
	"os"
	"strings"
	"testing"
)

func TestMarketUpdateRollbackClaimMigrationSealsSingleUseEvidence(t *testing.T) {
	up := readMarketRollbackClaimMigration(t, "0013_market_update_job_rollback_claims.up.sql")
	for _, required := range []string{
		"CREATE TABLE IF NOT EXISTS cc_market_update_job_rollback_claims",
		"rollback_admission_id text NOT NULL UNIQUE",
		"record_id text NOT NULL UNIQUE REFERENCES cc_market_update_jobs(record_id) ON DELETE RESTRICT",
		"REFERENCES cc_market_update_job_verification_journal(receipt_id) ON DELETE RESTRICT",
		"expected_state_version bigint NOT NULL CHECK (expected_state_version = 4)",
		"expected_journal_sequence bigint NOT NULL CHECK (expected_journal_sequence = 3)",
		"claim_sequence bigint NOT NULL CHECK (claim_sequence = expected_journal_sequence + 1)",
		"CHECK (single_use)",
		"CHECK (atomic_persistence_required)",
		"CHECK (fresh_revalidation_required)",
		"CHECK (preserve_user_data)",
		"CHECK (preserve_secret_material)",
		"CHECK (NOT execution_authorized)",
		"CHECK (NOT production_mutation_allowed)",
		"UNIQUE (record_id, claim_sequence)",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("0013 up migration lacks %q", required)
		}
	}
}

func TestMarketUpdateRollbackClaimMigrationBindsFailedVerification(t *testing.T) {
	up := readMarketRollbackClaimMigration(t, "0013_market_update_job_rollback_claims.up.sql")
	for _, required := range []string{
		"CREATE CONSTRAINT TRIGGER cc_market_update_job_rollback_claim_pair",
		"DEFERRABLE INITIALLY DEFERRED",
		"j.state = 'ROLLING_BACK'",
		"j.state_version = NEW.expected_state_version",
		"j.journal_sequence = NEW.expected_journal_sequence",
		"v.outcome = 'FAILED'",
		"v.from_state = 'VERIFYING'",
		"v.to_state = 'ROLLING_BACK'",
		"v.rollback_required_now",
		"NOT v.terminal",
		"NOT v.execution_authorized",
		"NOT v.production_mutation_allowed",
		"Market update rollback claim evidence is immutable",
		"Market update rollback claim does not match durable failed-verification evidence",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("0013 does not bind rollback claim evidence: missing %q", required)
		}
	}
}

func TestMarketUpdateRollbackClaimMigrationDowngradeFailsClosed(t *testing.T) {
	down := strings.ToUpper(readMarketRollbackClaimMigration(t, "0013_market_update_job_rollback_claims.down.sql"))
	for _, required := range []string{
		"EXISTS (SELECT 1 FROM CC_MARKET_UPDATE_JOB_ROLLBACK_CLAIMS LIMIT 1)",
		"RAISE EXCEPTION 'CANNOT DOWNGRADE WHILE MARKET UPDATE ROLLBACK CLAIM EVIDENCE EXISTS'",
		"DROP TRIGGER IF EXISTS CC_MARKET_UPDATE_JOB_ROLLBACK_CLAIM_PAIR",
		"DROP FUNCTION IF EXISTS CC_MARKET_VALIDATE_UPDATE_JOB_ROLLBACK_CLAIM()",
		"DROP TABLE IF EXISTS CC_MARKET_UPDATE_JOB_ROLLBACK_CLAIMS",
	} {
		if !strings.Contains(down, required) {
			t.Fatalf("0013 down migration lacks %q", required)
		}
	}
	if strings.Contains(down, "CASCADE") {
		t.Fatal("0013 down migration must not cascade")
	}
}

func TestMarketUpdateRollbackClaimMigrationIsMarketScoped(t *testing.T) {
	up := strings.ToUpper(readMarketRollbackClaimMigration(t, "0013_market_update_job_rollback_claims.up.sql"))
	for _, forbidden := range []string{
		"CC_LOCAL_USERS",
		"CC_AUTH_SESSIONS",
		"CC_CORE_OBJECTS",
		"CC_NODE_",
		"CASCADE",
	} {
		if strings.Contains(up, forbidden) {
			t.Fatalf("0013 Market migration widens scope with %q", forbidden)
		}
	}
}

func readMarketRollbackClaimMigration(t *testing.T, name string) string {
	t.Helper()
	content, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
