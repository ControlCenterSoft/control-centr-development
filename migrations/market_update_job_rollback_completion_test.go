package migrations

import (
	"os"
	"strings"
	"testing"
)

func TestMarketUpdateRollbackCompletionMigrationSealsAtomicAttemptConsumption(t *testing.T) {
	up := readMarketRollbackCompletionMigration(t, "0014_market_update_job_rollback_completion.up.sql")
	for _, required := range []string{
		"CREATE TABLE IF NOT EXISTS cc_market_update_job_rollback_completion_journal",
		"attempt_id text NOT NULL UNIQUE",
		"claim_id text NOT NULL UNIQUE",
		"REFERENCES cc_market_update_job_rollback_claims(claim_id) ON DELETE RESTRICT",
		"record_id text NOT NULL UNIQUE REFERENCES cc_market_update_jobs(record_id) ON DELETE RESTRICT",
		"CHECK (attempt_consumed)",
		"CHECK (atomic_persistence_required)",
		"CHECK (preserve_user_data)",
		"CHECK (preserve_secret_material)",
		"CHECK (terminal)",
		"CHECK (NOT further_attempt_authorized)",
		"CHECK (NOT automatic_retry_authorized)",
		"CHECK (NOT execution_authorized)",
		"CHECK (NOT generic_command_authorized)",
		"CHECK (NOT production_mutation_allowed)",
		"source_state jsonb NOT NULL CHECK (jsonb_typeof(source_state) = 'object')",
		"attempt jsonb NOT NULL CHECK (jsonb_typeof(attempt) = 'object')",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("0014 up migration lacks %q", required)
		}
	}
}

func TestMarketUpdateRollbackCompletionMigrationBindsTerminalLifecycle(t *testing.T) {
	up := readMarketRollbackCompletionMigration(t, "0014_market_update_job_rollback_completion.up.sql")
	for _, required := range []string{
		"from_state text NOT NULL CHECK (from_state = 'ROLLING_BACK')",
		"to_state_version bigint NOT NULL CHECK (to_state_version = from_state_version + 1)",
		"(outcome = 'SUCCEEDED' AND to_state = 'ROLLED_BACK' AND NOT recovery_required)",
		"(outcome = 'FAILED' AND to_state = 'FAILED' AND recovery_required)",
		"CREATE CONSTRAINT TRIGGER cc_market_update_job_rollback_completion_pair",
		"DEFERRABLE INITIALLY DEFERRED",
		"j.state = NEW.to_state",
		"j.state_version = NEW.to_state_version",
		"j.journal_sequence = NEW.journal_sequence",
		"c.claim_sequence + 1 = NEW.journal_sequence",
		"c.single_use",
		"v.outcome = 'FAILED'",
		"v.to_state = 'ROLLING_BACK'",
		"Market update rollback completion evidence is immutable",
		"Market update rollback completion does not match durable claim/job evidence",
		"Market update rollback completion attempt JSON lineage mismatch",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("0014 does not bind terminal rollback evidence: missing %q", required)
		}
	}
}

func TestMarketUpdateRollbackCompletionMigrationDowngradeFailsClosed(t *testing.T) {
	down := strings.ToUpper(readMarketRollbackCompletionMigration(t, "0014_market_update_job_rollback_completion.down.sql"))
	for _, required := range []string{
		"EXISTS (SELECT 1 FROM CC_MARKET_UPDATE_JOB_ROLLBACK_COMPLETION_JOURNAL LIMIT 1)",
		"RAISE EXCEPTION 'CANNOT DOWNGRADE WHILE MARKET UPDATE ROLLBACK COMPLETION EVIDENCE EXISTS'",
		"DROP TRIGGER IF EXISTS CC_MARKET_UPDATE_JOB_ROLLBACK_COMPLETION_PAIR",
		"DROP FUNCTION IF EXISTS CC_MARKET_VALIDATE_UPDATE_JOB_ROLLBACK_COMPLETION()",
		"DROP TABLE IF EXISTS CC_MARKET_UPDATE_JOB_ROLLBACK_COMPLETION_JOURNAL",
	} {
		if !strings.Contains(down, required) {
			t.Fatalf("0014 down migration lacks %q", required)
		}
	}
	if strings.Contains(down, "CASCADE") {
		t.Fatal("0014 down migration must not cascade")
	}
}

func TestMarketUpdateRollbackCompletionMigrationIsMarketScoped(t *testing.T) {
	up := strings.ToUpper(readMarketRollbackCompletionMigration(t, "0014_market_update_job_rollback_completion.up.sql"))
	for _, forbidden := range []string{
		"CC_LOCAL_USERS",
		"CC_AUTH_SESSIONS",
		"CC_CORE_OBJECTS",
		"CC_NODE_",
		"CASCADE",
	} {
		if strings.Contains(up, forbidden) {
			t.Fatalf("0014 Market migration widens scope with %q", forbidden)
		}
	}
}

func readMarketRollbackCompletionMigration(t *testing.T, name string) string {
	t.Helper()
	content, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
