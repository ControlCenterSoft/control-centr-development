BEGIN;

DROP INDEX IF EXISTS cc_deployment_step_dependencies_reverse_idx;
DROP TABLE IF EXISTS cc_deployment_step_dependencies;
DROP TABLE IF EXISTS cc_deployment_steps;
DROP INDEX IF EXISTS cc_deployment_plans_revision_state_idx;
DROP TABLE IF EXISTS cc_deployment_plans;
DROP INDEX IF EXISTS cc_architecture_validation_findings_severity_idx;
DROP TABLE IF EXISTS cc_architecture_validation_findings;
DROP INDEX IF EXISTS cc_architecture_validation_results_revision_idx;
DROP TABLE IF EXISTS cc_architecture_validation_results;
DROP INDEX IF EXISTS cc_solution_topology_edges_relation_idx;
DROP TABLE IF EXISTS cc_solution_topology_edges;
DROP INDEX IF EXISTS cc_solution_topology_nodes_provider_idx;
DROP TABLE IF EXISTS cc_solution_topology_nodes;
DROP INDEX IF EXISTS cc_solution_revisions_solution_state_idx;
DROP TABLE IF EXISTS cc_solution_revisions;
DROP INDEX IF EXISTS cc_infrastructure_solutions_scope_state_idx;
DROP TABLE IF EXISTS cc_infrastructure_solutions;
DROP INDEX IF EXISTS cc_provider_bindings_target_idx;
DROP INDEX IF EXISTS cc_provider_bindings_scope_idx;
DROP TABLE IF EXISTS cc_provider_bindings;
DROP INDEX IF EXISTS cc_provider_manifests_provider_idx;
DROP TABLE IF EXISTS cc_provider_manifests;

COMMIT;
