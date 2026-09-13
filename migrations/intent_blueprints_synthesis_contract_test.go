package migrations

import (
	"strings"
	"testing"
)

func TestIntentBlueprintSynthesisMigrationIsAdditiveAndScoped(t *testing.T) {
	up := readMigration(t, "0015_intent_blueprints_synthesis.up.sql")
	for _, required := range []string{
		"CREATE TABLE IF NOT EXISTS cc_infrastructure_intents",
		"CREATE TABLE IF NOT EXISTS cc_intent_revisions",
		"CREATE TABLE IF NOT EXISTS cc_solution_blueprints",
		"CREATE TABLE IF NOT EXISTS cc_blueprint_revisions",
		"CREATE TABLE IF NOT EXISTS cc_synthesis_assessments",
		"CREATE TABLE IF NOT EXISTS cc_synthesis_candidates",
		"CREATE TABLE IF NOT EXISTS cc_synthesis_constraint_results",
		"CREATE TABLE IF NOT EXISTS cc_synthesis_decisions",
		"CREATE TABLE IF NOT EXISTS cc_expansion_assessments",
		"CREATE TABLE IF NOT EXISTS cc_expansion_options",
		"input_snapshot_digest ~ '^sha256:[0-9a-f]{64}$'",
		"blueprint_digest ~ '^sha256:[0-9a-f]{64}$'",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("0015 up migration lacks %q", required)
		}
	}

	upper := strings.ToUpper(up)
	for _, protected := range []string{
		"CC_PROVIDER_MANIFESTS",
		"CC_PROVIDER_BINDINGS",
		"CC_INFRASTRUCTURE_SOLUTIONS",
		"CC_SOLUTION_REVISIONS",
		"CC_LOCAL_USERS",
		"CC_AUDIT_EVENTS",
		"CC_CHANGES",
		"CC_JOBS",
		"CC_INCIDENT_READ_MODELS",
	} {
		if strings.Contains(upper, "ALTER TABLE "+protected) || strings.Contains(upper, "DROP TABLE "+protected) {
			t.Fatalf("0015 upgrade mutates protected prior table %s", protected)
		}
	}
}

func TestIntentBlueprintSynthesisDownMigrationIsScoped(t *testing.T) {
	down := strings.ToUpper(readMigration(t, "0015_intent_blueprints_synthesis.down.sql"))
	for _, created := range []string{
		"CC_EXPANSION_OPTIONS",
		"CC_EXPANSION_ASSESSMENTS",
		"CC_SYNTHESIS_DECISIONS",
		"CC_SYNTHESIS_CONSTRAINT_RESULTS",
		"CC_SYNTHESIS_CANDIDATES",
		"CC_SYNTHESIS_ASSESSMENTS",
		"CC_BLUEPRINT_REVISIONS",
		"CC_SOLUTION_BLUEPRINTS",
		"CC_INTENT_REVISIONS",
		"CC_INFRASTRUCTURE_INTENTS",
	} {
		if !strings.Contains(down, "DROP TABLE IF EXISTS "+created) {
			t.Errorf("0015 down migration does not remove %s", created)
		}
	}
	for _, protected := range []string{
		"CC_PROVIDER_MANIFESTS",
		"CC_PROVIDER_BINDINGS",
		"CC_INFRASTRUCTURE_SOLUTIONS",
		"CC_SOLUTION_REVISIONS",
		"CC_LOCAL_USERS",
		"CC_AUDIT_EVENTS",
		"CC_CHANGES",
		"CC_JOBS",
		"CC_INCIDENT_READ_MODELS",
	} {
		if strings.Contains(down, "DROP TABLE IF EXISTS "+protected) || strings.Contains(down, "ALTER TABLE "+protected) {
			t.Fatalf("0015 down migration touches protected prior table %s", protected)
		}
	}
}
