BEGIN;

DROP INDEX IF EXISTS cc_expansion_options_status_idx;
DROP TABLE IF EXISTS cc_expansion_options;
DROP INDEX IF EXISTS cc_expansion_assessments_revision_state_idx;
DROP TABLE IF EXISTS cc_expansion_assessments;
DROP INDEX IF EXISTS cc_synthesis_decisions_candidate_idx;
DROP TABLE IF EXISTS cc_synthesis_decisions;
DROP INDEX IF EXISTS cc_synthesis_constraint_results_status_idx;
DROP TABLE IF EXISTS cc_synthesis_constraint_results;
DROP INDEX IF EXISTS cc_synthesis_candidates_assessment_status_idx;
DROP TABLE IF EXISTS cc_synthesis_candidates;
DROP INDEX IF EXISTS cc_synthesis_assessments_input_idx;
DROP TABLE IF EXISTS cc_synthesis_assessments;
DROP INDEX IF EXISTS cc_blueprint_revisions_blueprint_status_idx;
DROP TABLE IF EXISTS cc_blueprint_revisions;
DROP INDEX IF EXISTS cc_solution_blueprints_scope_name_idx;
DROP TABLE IF EXISTS cc_solution_blueprints;
DROP INDEX IF EXISTS cc_intent_revisions_intent_state_idx;
DROP TABLE IF EXISTS cc_intent_revisions;
DROP INDEX IF EXISTS cc_infrastructure_intents_scope_name_idx;
DROP TABLE IF EXISTS cc_infrastructure_intents;

COMMIT;
