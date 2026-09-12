package migrations

import (
	"os"
	"strings"
	"testing"
)

func TestMarketUpdateApplyReceiptMigrationSealsAtomicVerifyingPair(t *testing.T) {
	up := readMarketApplyMigration(t, "0011_market_update_job_apply_receipts.up.sql")
	for _, required := range []string{
		"state IN ('PREPARED', 'RUNNING', 'VERIFYING')",
		"state = 'VERIFYING'",
		"state_version = 3",
		"journal_sequence = 2",
		"CREATE TABLE IF NOT EXISTS cc_market_update_job_apply_journal",
		"CHECK (preserve_user_data)",
		"CHECK (preserve_secret_material)",
		"CHECK (verification_required)",
		"CHECK (rollback_required_on_failure)",
		"CHECK (NOT execution_authorized)",
		"CHECK (NOT production_mutation_allowed)",
		"CREATE CONSTRAINT TRIGGER cc_market_update_job_apply_journal_pair",
		"DEFERRABLE INITIALLY DEFERRED",
		"VERIFYING Market update job requires exactly one apply receipt",
		"Market update VERIFYING record and apply receipt evidence do not match",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("0011 up migration lacks %q", required)
		}
	}
}

func TestMarketUpdateApplyReceiptMigrationPreservesSnapshotAndRollbackSafety(t *testing.T) {
	up := readMarketApplyMigration(t, "0011_market_update_job_apply_receipts.up.sql")
	for _, required := range []string{
		"require_pre_migration_snapshot AND pre_migration_snapshot_id IS NOT NULL",
		"NOT require_pre_migration_snapshot AND pre_migration_snapshot_id IS NULL",
		"rollback_required_on_failure boolean NOT NULL DEFAULT true",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("0011 does not preserve apply rollback safety: missing %q", required)
		}
	}

	down := strings.ToUpper(readMarketApplyMigration(t, "0011_market_update_job_apply_receipts.down.sql"))
	for _, required := range []string{
		"EXISTS (SELECT 1 FROM CC_MARKET_UPDATE_JOB_APPLY_JOURNAL LIMIT 1)",
		"EXISTS (SELECT 1 FROM CC_MARKET_UPDATE_JOBS WHERE STATE = 'VERIFYING' LIMIT 1)",
		"RAISE EXCEPTION 'CANNOT DOWNGRADE WHILE MARKET UPDATE APPLY RECEIPT STATE EXISTS'",
		"DROP TABLE IF EXISTS CC_MARKET_UPDATE_JOB_APPLY_JOURNAL",
		"CHECK (STATE IN ('PREPARED', 'RUNNING'))",
	} {
		if !strings.Contains(down, required) {
			t.Fatalf("0011 down migration lacks %q", required)
		}
	}
	if strings.Contains(down, "CASCADE") {
		t.Fatal("0011 down migration must not cascade")
	}
}

func TestMarketUpdateApplyReceiptMigrationIsMarketScoped(t *testing.T) {
	up := strings.ToUpper(readMarketApplyMigration(t, "0011_market_update_job_apply_receipts.up.sql"))
	for _, forbidden := range []string{
		"CC_LOCAL_USERS",
		"CC_AUTH_SESSIONS",
		"CC_CORE_OBJECTS",
		"CC_NODE_",
		"CASCADE",
	} {
		if strings.Contains(up, forbidden) {
			t.Fatalf("0011 Market migration widens scope with %q", forbidden)
		}
	}
}

func readMarketApplyMigration(t *testing.T, name string) string {
	t.Helper()
	content, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
