package migrations

import (
	"strings"
	"testing"
)

func TestProviderSolutionCoreMigrationIsAdditiveAndScoped(t *testing.T) {
	up := readMigration(t, "0014_provider_solution_core.up.sql")
	for _, required := range []string{
		"CREATE TABLE IF NOT EXISTS cc_provider_manifests",
		"CREATE TABLE IF NOT EXISTS cc_provider_bindings",
		"CREATE TABLE IF NOT EXISTS cc_infrastructure_solutions",
		"CREATE TABLE IF NOT EXISTS cc_solution_revisions",
		"CREATE TABLE IF NOT EXISTS cc_solution_topology_nodes",
		"CREATE TABLE IF NOT EXISTS cc_solution_topology_edges",
		"CREATE TABLE IF NOT EXISTS cc_architecture_validation_results",
		"CREATE TABLE IF NOT EXISTS cc_architecture_validation_findings",
		"CREATE TABLE IF NOT EXISTS cc_deployment_plans",
		"CREATE TABLE IF NOT EXISTS cc_deployment_steps",
		"CREATE TABLE IF NOT EXISTS cc_deployment_step_dependencies",
		"resource_version   varchar(255) NOT NULL UNIQUE",
		"topology_digest ~ '^sha256:[0-9a-f]{64}$'",
		"plan_digest ~ '^sha256:[0-9a-f]{64}$'",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("0014 up migration lacks %q", required)
		}
	}

	upper := strings.ToUpper(up)
	for _, protected := range []string{
		"ORGANIZATIONS",
		"RESOURCES",
		"CONFIG_REVISIONS",
		"CC_LOCAL_USERS",
		"CC_AUDIT_EVENTS",
		"CC_CHANGES",
		"CC_JOBS",
		"CC_INCIDENT_READ_MODELS",
	} {
		if strings.Contains(upper, "ALTER TABLE "+protected) || strings.Contains(upper, "DROP TABLE "+protected) {
			t.Fatalf("0014 upgrade mutates protected prior-release table %s", protected)
		}
	}
}

func TestProviderSolutionCoreDownMigrationRemovesOnly0014Objects(t *testing.T) {
	down := strings.ToUpper(readMigration(t, "0014_provider_solution_core.down.sql"))
	for _, created := range []string{
		"CC_PROVIDER_MANIFESTS",
		"CC_PROVIDER_BINDINGS",
		"CC_INFRASTRUCTURE_SOLUTIONS",
		"CC_SOLUTION_REVISIONS",
		"CC_SOLUTION_TOPOLOGY_NODES",
		"CC_SOLUTION_TOPOLOGY_EDGES",
		"CC_ARCHITECTURE_VALIDATION_RESULTS",
		"CC_ARCHITECTURE_VALIDATION_FINDINGS",
		"CC_DEPLOYMENT_PLANS",
		"CC_DEPLOYMENT_STEPS",
		"CC_DEPLOYMENT_STEP_DEPENDENCIES",
	} {
		if !strings.Contains(down, "DROP TABLE IF EXISTS "+created) {
			t.Errorf("0014 down migration does not remove %s", created)
		}
	}
	for _, protected := range []string{
		"ORGANIZATIONS",
		"RESOURCES",
		"CONFIG_REVISIONS",
		"CC_LOCAL_USERS",
		"CC_AUDIT_EVENTS",
		"CC_CHANGES",
		"CC_JOBS",
		"CC_INCIDENT_READ_MODELS",
	} {
		if strings.Contains(down, "DROP TABLE IF EXISTS "+protected) || strings.Contains(down, "ALTER TABLE "+protected) {
			t.Fatalf("0014 down migration touches protected prior-release table %s", protected)
		}
	}
}
