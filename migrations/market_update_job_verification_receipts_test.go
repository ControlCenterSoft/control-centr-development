package migrations

import (
	"os"
	"strings"
	"testing"
)

func TestMarketUpdateVerificationReceiptMigrationSealsAtomicCompletion(t *testing.T) {
	up := readMarketVerificationMigration(t, "0012_market_update_job_verification_receipts.up.sql")
	for _, required := range []string{
		"state IN ('PREPARED', 'RUNNING', 'VERIFYING', 'SUCCEEDED', 'ROLLING_BACK')",
		"state IN ('SUCCEEDED', 'ROLLING_BACK')",
		"state_version = 4",
		"journal_sequence = 3",
		"CREATE TABLE IF NOT EXISTS cc_market_update_job_verification_journal",
		"outcome IN ('PASSED', 'FAILED')",
		"CHECK (preserve_user_data)",
		"CHECK (preserve_secret_material)",
		"CHECK (NOT execution_authorized)",
		"CHECK (NOT production_mutation_allowed)",
		"outcome = 'PASSED'",
		"to_state = 'SUCCEEDED'",
		"outcome = 'FAILED'",
		"to_state = 'ROLLING_BACK'",
		"UNIQUE (record_id, apply_receipt_id)",
		"CREATE CONSTRAINT TRIGGER cc_market_update_job_verification_journal_pair",
		"CREATE CONSTRAINT TRIGGER cc_market_update_job_state_pair",
		"DEFERRABLE INITIALLY DEFERRED",
		"completed Market update verification requires exactly one receipt",
		"Market update verification record and receipt evidence do not match",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("0012 up migration lacks %q", required)
		}
	}
}

func TestMarketUpdateVerificationReceiptMigrationPreservesRollbackSafety(t *testing.T) {
	up := readMarketVerificationMigration(t, "0012_market_update_job_verification_receipts.up.sql")
	for _, required := range []string{
		"REFERENCES cc_market_update_job_apply_journal(receipt_id) ON DELETE RESTRICT",
		"AND NOT rollback_required_now",
		"AND rollback_required_now",
		"AND v.preserve_user_data",
		"AND v.preserve_secret_material",
		"AND NOT v.execution_authorized",
		"AND NOT v.production_mutation_allowed",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("0012 does not preserve verification safety: missing %q", required)
		}
	}
}

func TestMarketUpdateVerificationReceiptMigrationDowngradeFailsClosed(t *testing.T) {
	down := strings.ToUpper(readMarketVerificationMigration(t, "0012_market_update_job_verification_receipts.down.sql"))
	for _, required := range []string{
		"EXISTS (SELECT 1 FROM CC_MARKET_UPDATE_JOB_VERIFICATION_JOURNAL LIMIT 1)",
		"WHERE STATE IN ('SUCCEEDED', 'ROLLING_BACK')",
		"RAISE EXCEPTION 'CANNOT DOWNGRADE WHILE MARKET UPDATE VERIFICATION RECEIPT STATE EXISTS'",
		"DROP TABLE IF EXISTS CC_MARKET_UPDATE_JOB_VERIFICATION_JOURNAL",
		"CHECK (STATE IN ('PREPARED', 'RUNNING', 'VERIFYING'))",
	} {
		if !strings.Contains(down, required) {
			t.Fatalf("0012 down migration lacks %q", required)
		}
	}
	if strings.Contains(down, "CASCADE") {
		t.Fatal("0012 down migration must not cascade")
	}
}

func TestMarketUpdateVerificationReceiptMigrationIsMarketScoped(t *testing.T) {
	up := strings.ToUpper(readMarketVerificationMigration(t, "0012_market_update_job_verification_receipts.up.sql"))
	for _, forbidden := range []string{
		"CC_LOCAL_USERS",
		"CC_AUTH_SESSIONS",
		"CC_CORE_OBJECTS",
		"CC_NODE_",
		"CASCADE",
	} {
		if strings.Contains(up, forbidden) {
			t.Fatalf("0012 Market migration widens scope with %q", forbidden)
		}
	}
}

func readMarketVerificationMigration(t *testing.T, name string) string {
	t.Helper()
	content, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
