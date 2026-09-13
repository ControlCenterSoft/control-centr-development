BEGIN;

CREATE TABLE IF NOT EXISTS cc_infrastructure_intents (
    object_id            varchar(255) PRIMARY KEY,
    scope_id             varchar(255) NOT NULL,
    owner_scope          varchar(255) NOT NULL,
    generation           bigint       NOT NULL CHECK (generation > 0),
    resource_version     varchar(255) NOT NULL UNIQUE,
    display_name         varchar(160) NOT NULL,
    current_revision_id  varchar(255),
    document             jsonb        NOT NULL,
    created_at           timestamptz  NOT NULL,
    updated_at           timestamptz  NOT NULL,
    CONSTRAINT cc_infrastructure_intents_object_id_format CHECK (
        object_id ~ '^[A-Za-z0-9](?:[A-Za-z0-9._:-]{0,253}[A-Za-z0-9])?$'
    ),
    CONSTRAINT cc_infrastructure_intents_scope_id_format CHECK (
        scope_id ~ '^[A-Za-z0-9](?:[A-Za-z0-9._:-]{0,253}[A-Za-z0-9])?$'
    ),
    CONSTRAINT cc_infrastructure_intents_owner_scope_format CHECK (
        owner_scope ~ '^[A-Za-z0-9](?:[A-Za-z0-9._:-]{0,253}[A-Za-z0-9])?$'
    ),
    CONSTRAINT cc_infrastructure_intents_resource_version_format CHECK (
        length(resource_version) BETWEEN 1 AND 255 AND resource_version !~ '[[:space:]]'
    ),
    CONSTRAINT cc_infrastructure_intents_display_name_nonempty CHECK (length(display_name) > 0),
    CONSTRAINT cc_infrastructure_intents_document_object CHECK (jsonb_typeof(document) = 'object'),
    CONSTRAINT cc_infrastructure_intents_time_order CHECK (updated_at >= created_at)
);

CREATE INDEX IF NOT EXISTS cc_infrastructure_intents_scope_name_idx
    ON cc_infrastructure_intents (scope_id, display_name, object_id);

CREATE TABLE IF NOT EXISTS cc_intent_revisions (
    object_id           varchar(255) PRIMARY KEY,
    scope_id            varchar(255) NOT NULL,
    owner_scope         varchar(255) NOT NULL,
    generation          bigint       NOT NULL CHECK (generation > 0),
    resource_version    varchar(255) NOT NULL UNIQUE,
    intent_id           varchar(255) NOT NULL REFERENCES cc_infrastructure_intents(object_id) ON DELETE CASCADE,
    ordinal             bigint       NOT NULL CHECK (ordinal > 0),
    state               text         NOT NULL CHECK (state IN (
        'draft', 'normalized', 'needs_clarification', 'confirmed', 'feasibility_checked',
        'satisfied', 'partially_satisfied', 'unsatisfiable', 'stale', 'superseded'
    )),
    document            jsonb        NOT NULL,
    created_at          timestamptz  NOT NULL,
    updated_at          timestamptz  NOT NULL,
    CONSTRAINT cc_intent_revisions_object_id_format CHECK (
        object_id ~ '^[A-Za-z0-9](?:[A-Za-z0-9._:-]{0,253}[A-Za-z0-9])?$'
    ),
    CONSTRAINT cc_intent_revisions_scope_id_format CHECK (
        scope_id ~ '^[A-Za-z0-9](?:[A-Za-z0-9._:-]{0,253}[A-Za-z0-9])?$'
    ),
    CONSTRAINT cc_intent_revisions_owner_scope_format CHECK (
        owner_scope ~ '^[A-Za-z0-9](?:[A-Za-z0-9._:-]{0,253}[A-Za-z0-9])?$'
    ),
    CONSTRAINT cc_intent_revisions_resource_version_format CHECK (
        length(resource_version) BETWEEN 1 AND 255 AND resource_version !~ '[[:space:]]'
    ),
    CONSTRAINT cc_intent_revisions_document_object CHECK (jsonb_typeof(document) = 'object'),
    CONSTRAINT cc_intent_revisions_time_order CHECK (updated_at >= created_at),
    UNIQUE (intent_id, ordinal)
);

CREATE INDEX IF NOT EXISTS cc_intent_revisions_intent_state_idx
    ON cc_intent_revisions (intent_id, state, ordinal DESC);

CREATE TABLE IF NOT EXISTS cc_solution_blueprints (
    object_id            varchar(255) PRIMARY KEY,
    scope_id             varchar(255) NOT NULL,
    owner_scope          varchar(255) NOT NULL,
    generation           bigint       NOT NULL CHECK (generation > 0),
    resource_version     varchar(255) NOT NULL UNIQUE,
    display_name         varchar(160) NOT NULL,
    current_revision_id  varchar(255),
    document             jsonb        NOT NULL,
    created_at           timestamptz  NOT NULL,
    updated_at           timestamptz  NOT NULL,
    CONSTRAINT cc_solution_blueprints_object_id_format CHECK (
        object_id ~ '^[A-Za-z0-9](?:[A-Za-z0-9._:-]{0,253}[A-Za-z0-9])?$'
    ),
    CONSTRAINT cc_solution_blueprints_scope_id_format CHECK (
        scope_id ~ '^[A-Za-z0-9](?:[A-Za-z0-9._:-]{0,253}[A-Za-z0-9])?$'
    ),
    CONSTRAINT cc_solution_blueprints_owner_scope_format CHECK (
        owner_scope ~ '^[A-Za-z0-9](?:[A-Za-z0-9._:-]{0,253}[A-Za-z0-9])?$'
    ),
    CONSTRAINT cc_solution_blueprints_resource_version_format CHECK (
        length(resource_version) BETWEEN 1 AND 255 AND resource_version !~ '[[:space:]]'
    ),
    CONSTRAINT cc_solution_blueprints_display_name_nonempty CHECK (length(display_name) > 0),
    CONSTRAINT cc_solution_blueprints_document_object CHECK (jsonb_typeof(document) = 'object'),
    CONSTRAINT cc_solution_blueprints_time_order CHECK (updated_at >= created_at)
);

CREATE INDEX IF NOT EXISTS cc_solution_blueprints_scope_name_idx
    ON cc_solution_blueprints (scope_id, display_name, object_id);

CREATE TABLE IF NOT EXISTS cc_blueprint_revisions (
    object_id             varchar(255) PRIMARY KEY,
    scope_id              varchar(255) NOT NULL,
    owner_scope           varchar(255) NOT NULL,
    generation            bigint       NOT NULL CHECK (generation > 0),
    resource_version      varchar(255) NOT NULL UNIQUE,
    blueprint_id          varchar(255) NOT NULL REFERENCES cc_solution_blueprints(object_id) ON DELETE CASCADE,
    ordinal               bigint       NOT NULL CHECK (ordinal > 0),
    qualification_status  text         NOT NULL CHECK (qualification_status IN ('draft', 'qualified', 'deprecated')),
    blueprint_digest      varchar(71)  NOT NULL,
    signature_ref         varchar(255),
    document              jsonb        NOT NULL,
    created_at            timestamptz  NOT NULL,
    updated_at            timestamptz  NOT NULL,
    CONSTRAINT cc_blueprint_revisions_object_id_format CHECK (
        object_id ~ '^[A-Za-z0-9](?:[A-Za-z0-9._:-]{0,253}[A-Za-z0-9])?$'
    ),
    CONSTRAINT cc_blueprint_revisions_scope_id_format CHECK (
        scope_id ~ '^[A-Za-z0-9](?:[A-Za-z0-9._:-]{0,253}[A-Za-z0-9])?$'
    ),
    CONSTRAINT cc_blueprint_revisions_owner_scope_format CHECK (
        owner_scope ~ '^[A-Za-z0-9](?:[A-Za-z0-9._:-]{0,253}[A-Za-z0-9])?$'
    ),
    CONSTRAINT cc_blueprint_revisions_resource_version_format CHECK (
        length(resource_version) BETWEEN 1 AND 255 AND resource_version !~ '[[:space:]]'
    ),
    CONSTRAINT cc_blueprint_revisions_digest_format CHECK (blueprint_digest ~ '^sha256:[0-9a-f]{64}$'),
    CONSTRAINT cc_blueprint_revisions_qualified_signature CHECK (
        qualification_status <> 'qualified' OR (signature_ref IS NOT NULL AND length(signature_ref) > 0)
    ),
    CONSTRAINT cc_blueprint_revisions_document_object CHECK (jsonb_typeof(document) = 'object'),
    CONSTRAINT cc_blueprint_revisions_time_order CHECK (updated_at >= created_at),
    UNIQUE (blueprint_id, ordinal),
    UNIQUE (blueprint_id, blueprint_digest)
);

CREATE INDEX IF NOT EXISTS cc_blueprint_revisions_blueprint_status_idx
    ON cc_blueprint_revisions (blueprint_id, qualification_status, ordinal DESC);

CREATE TABLE IF NOT EXISTS cc_synthesis_assessments (
    object_id                varchar(255) PRIMARY KEY,
    scope_id                 varchar(255) NOT NULL,
    owner_scope               varchar(255) NOT NULL,
    generation                bigint       NOT NULL CHECK (generation > 0),
    resource_version          varchar(255) NOT NULL UNIQUE,
    intent_revision_id        varchar(255) NOT NULL REFERENCES cc_intent_revisions(object_id) ON DELETE RESTRICT,
    blueprint_revision_id     varchar(255) NOT NULL REFERENCES cc_blueprint_revisions(object_id) ON DELETE RESTRICT,
    input_snapshot_digest     varchar(71)  NOT NULL,
    provider_catalog_digest   varchar(71)  NOT NULL,
    policy_revision           varchar(255) NOT NULL,
    state                     text         NOT NULL CHECK (state IN ('pending', 'running', 'completed', 'stale', 'failed')),
    evaluated_at              timestamptz,
    document                  jsonb        NOT NULL,
    created_at                timestamptz  NOT NULL,
    updated_at                timestamptz  NOT NULL,
    CONSTRAINT cc_synthesis_assessments_object_id_format CHECK (
        object_id ~ '^[A-Za-z0-9](?:[A-Za-z0-9._:-]{0,253}[A-Za-z0-9])?$'
    ),
    CONSTRAINT cc_synthesis_assessments_scope_id_format CHECK (
        scope_id ~ '^[A-Za-z0-9](?:[A-Za-z0-9._:-]{0,253}[A-Za-z0-9])?$'
    ),
    CONSTRAINT cc_synthesis_assessments_owner_scope_format CHECK (
        owner_scope ~ '^[A-Za-z0-9](?:[A-Za-z0-9._:-]{0,253}[A-Za-z0-9])?$'
    ),
    CONSTRAINT cc_synthesis_assessments_resource_version_format CHECK (
        length(resource_version) BETWEEN 1 AND 255 AND resource_version !~ '[[:space:]]'
    ),
    CONSTRAINT cc_synthesis_assessments_input_digest_format CHECK (input_snapshot_digest ~ '^sha256:[0-9a-f]{64}$'),
    CONSTRAINT cc_synthesis_assessments_catalog_digest_format CHECK (provider_catalog_digest ~ '^sha256:[0-9a-f]{64}$'),
    CONSTRAINT cc_synthesis_assessments_completed_time CHECK (state <> 'completed' OR evaluated_at IS NOT NULL),
    CONSTRAINT cc_synthesis_assessments_document_object CHECK (jsonb_typeof(document) = 'object'),
    CONSTRAINT cc_synthesis_assessments_time_order CHECK (updated_at >= created_at)
);

CREATE INDEX IF NOT EXISTS cc_synthesis_assessments_input_idx
    ON cc_synthesis_assessments (intent_revision_id, blueprint_revision_id, state, created_at DESC, object_id);

CREATE TABLE IF NOT EXISTS cc_synthesis_candidates (
    object_id              varchar(255) PRIMARY KEY,
    scope_id               varchar(255) NOT NULL,
    owner_scope            varchar(255) NOT NULL,
    generation             bigint       NOT NULL CHECK (generation > 0),
    resource_version       varchar(255) NOT NULL UNIQUE,
    assessment_id          varchar(255) NOT NULL REFERENCES cc_synthesis_assessments(object_id) ON DELETE CASCADE,
    status                 text         NOT NULL CHECK (status IN ('valid', 'valid_with_warning', 'blocked')),
    topology_digest        varchar(71)  NOT NULL,
    score                  double precision CHECK (score IS NULL OR (score >= 0 AND score <= 1)),
    document               jsonb        NOT NULL,
    created_at             timestamptz  NOT NULL,
    updated_at             timestamptz  NOT NULL,
    CONSTRAINT cc_synthesis_candidates_object_id_format CHECK (
        object_id ~ '^[A-Za-z0-9](?:[A-Za-z0-9._:-]{0,253}[A-Za-z0-9])?$'
    ),
    CONSTRAINT cc_synthesis_candidates_scope_id_format CHECK (
        scope_id ~ '^[A-Za-z0-9](?:[A-Za-z0-9._:-]{0,253}[A-Za-z0-9])?$'
    ),
    CONSTRAINT cc_synthesis_candidates_owner_scope_format CHECK (
        owner_scope ~ '^[A-Za-z0-9](?:[A-Za-z0-9._:-]{0,253}[A-Za-z0-9])?$'
    ),
    CONSTRAINT cc_synthesis_candidates_resource_version_format CHECK (
        length(resource_version) BETWEEN 1 AND 255 AND resource_version !~ '[[:space:]]'
    ),
    CONSTRAINT cc_synthesis_candidates_topology_digest_format CHECK (topology_digest ~ '^sha256:[0-9a-f]{64}$'),
    CONSTRAINT cc_synthesis_candidates_document_object CHECK (jsonb_typeof(document) = 'object'),
    CONSTRAINT cc_synthesis_candidates_time_order CHECK (updated_at >= created_at)
);

CREATE INDEX IF NOT EXISTS cc_synthesis_candidates_assessment_status_idx
    ON cc_synthesis_candidates (assessment_id, status, score DESC NULLS LAST, object_id);

CREATE TABLE IF NOT EXISTS cc_synthesis_constraint_results (
    candidate_id       varchar(255) NOT NULL REFERENCES cc_synthesis_candidates(object_id) ON DELETE CASCADE,
    requirement_id     varchar(255) NOT NULL,
    status             text         NOT NULL CHECK (status IN ('pass', 'warning', 'blocked', 'unknown')),
    reason             text         NOT NULL,
    document           jsonb        NOT NULL DEFAULT '{}'::jsonb,
    PRIMARY KEY (candidate_id, requirement_id),
    CONSTRAINT cc_synthesis_constraint_results_requirement_nonempty CHECK (length(requirement_id) > 0),
    CONSTRAINT cc_synthesis_constraint_results_reason_nonempty CHECK (length(reason) > 0),
    CONSTRAINT cc_synthesis_constraint_results_document_object CHECK (jsonb_typeof(document) = 'object')
);

CREATE INDEX IF NOT EXISTS cc_synthesis_constraint_results_status_idx
    ON cc_synthesis_constraint_results (status, candidate_id, requirement_id);

CREATE TABLE IF NOT EXISTS cc_synthesis_decisions (
    object_id                       varchar(255) PRIMARY KEY,
    scope_id                        varchar(255) NOT NULL,
    owner_scope                     varchar(255) NOT NULL,
    generation                      bigint       NOT NULL CHECK (generation > 0),
    resource_version                varchar(255) NOT NULL UNIQUE,
    assessment_id                   varchar(255) NOT NULL REFERENCES cc_synthesis_assessments(object_id) ON DELETE RESTRICT,
    selected_candidate_id           varchar(255) NOT NULL REFERENCES cc_synthesis_candidates(object_id) ON DELETE RESTRICT,
    rationale                       text         NOT NULL,
    resulting_solution_revision_id  varchar(255) REFERENCES cc_solution_revisions(object_id) ON DELETE SET NULL,
    decided_at                      timestamptz  NOT NULL,
    document                        jsonb        NOT NULL,
    created_at                      timestamptz  NOT NULL,
    updated_at                      timestamptz  NOT NULL,
    CONSTRAINT cc_synthesis_decisions_object_id_format CHECK (
        object_id ~ '^[A-Za-z0-9](?:[A-Za-z0-9._:-]{0,253}[A-Za-z0-9])?$'
    ),
    CONSTRAINT cc_synthesis_decisions_scope_id_format CHECK (
        scope_id ~ '^[A-Za-z0-9](?:[A-Za-z0-9._:-]{0,253}[A-Za-z0-9])?$'
    ),
    CONSTRAINT cc_synthesis_decisions_owner_scope_format CHECK (
        owner_scope ~ '^[A-Za-z0-9](?:[A-Za-z0-9._:-]{0,253}[A-Za-z0-9])?$'
    ),
    CONSTRAINT cc_synthesis_decisions_resource_version_format CHECK (
        length(resource_version) BETWEEN 1 AND 255 AND resource_version !~ '[[:space:]]'
    ),
    CONSTRAINT cc_synthesis_decisions_rationale_nonempty CHECK (length(rationale) > 0),
    CONSTRAINT cc_synthesis_decisions_document_object CHECK (jsonb_typeof(document) = 'object'),
    CONSTRAINT cc_synthesis_decisions_time_order CHECK (updated_at >= created_at),
    UNIQUE (assessment_id)
);

CREATE INDEX IF NOT EXISTS cc_synthesis_decisions_candidate_idx
    ON cc_synthesis_decisions (selected_candidate_id, decided_at DESC, object_id);

CREATE TABLE IF NOT EXISTS cc_expansion_assessments (
    object_id                 varchar(255) PRIMARY KEY,
    scope_id                  varchar(255) NOT NULL,
    owner_scope               varchar(255) NOT NULL,
    generation                bigint       NOT NULL CHECK (generation > 0),
    resource_version          varchar(255) NOT NULL UNIQUE,
    solution_revision_id      varchar(255) NOT NULL REFERENCES cc_solution_revisions(object_id) ON DELETE RESTRICT,
    input_snapshot_digest     varchar(71)  NOT NULL,
    state                     text         NOT NULL CHECK (state IN ('pending', 'running', 'completed', 'stale', 'failed')),
    evaluated_at              timestamptz,
    document                  jsonb        NOT NULL,
    created_at                timestamptz  NOT NULL,
    updated_at                timestamptz  NOT NULL,
    CONSTRAINT cc_expansion_assessments_object_id_format CHECK (
        object_id ~ '^[A-Za-z0-9](?:[A-Za-z0-9._:-]{0,253}[A-Za-z0-9])?$'
    ),
    CONSTRAINT cc_expansion_assessments_scope_id_format CHECK (
        scope_id ~ '^[A-Za-z0-9](?:[A-Za-z0-9._:-]{0,253}[A-Za-z0-9])?$'
    ),
    CONSTRAINT cc_expansion_assessments_owner_scope_format CHECK (
        owner_scope ~ '^[A-Za-z0-9](?:[A-Za-z0-9._:-]{0,253}[A-Za-z0-9])?$'
    ),
    CONSTRAINT cc_expansion_assessments_resource_version_format CHECK (
        length(resource_version) BETWEEN 1 AND 255 AND resource_version !~ '[[:space:]]'
    ),
    CONSTRAINT cc_expansion_assessments_input_digest_format CHECK (input_snapshot_digest ~ '^sha256:[0-9a-f]{64}$'),
    CONSTRAINT cc_expansion_assessments_completed_time CHECK (state <> 'completed' OR evaluated_at IS NOT NULL),
    CONSTRAINT cc_expansion_assessments_document_object CHECK (jsonb_typeof(document) = 'object'),
    CONSTRAINT cc_expansion_assessments_time_order CHECK (updated_at >= created_at)
);

CREATE INDEX IF NOT EXISTS cc_expansion_assessments_revision_state_idx
    ON cc_expansion_assessments (solution_revision_id, state, created_at DESC, object_id);

CREATE TABLE IF NOT EXISTS cc_expansion_options (
    assessment_id  varchar(255) NOT NULL REFERENCES cc_expansion_assessments(object_id) ON DELETE CASCADE,
    strategy       text         NOT NULL CHECK (strategy IN (
        'scale_up', 'scale_out', 'new_instance', 'new_cluster', 'split_migrate', 'replace', 'no_change'
    )),
    status         text         NOT NULL CHECK (status IN ('valid', 'valid_with_warning', 'blocked')),
    reason         text         NOT NULL,
    plan_digest    varchar(71),
    document       jsonb        NOT NULL DEFAULT '{}'::jsonb,
    PRIMARY KEY (assessment_id, strategy),
    CONSTRAINT cc_expansion_options_reason_nonempty CHECK (length(reason) > 0),
    CONSTRAINT cc_expansion_options_plan_digest_format CHECK (
        plan_digest IS NULL OR plan_digest ~ '^sha256:[0-9a-f]{64}$'
    ),
    CONSTRAINT cc_expansion_options_document_object CHECK (jsonb_typeof(document) = 'object')
);

CREATE INDEX IF NOT EXISTS cc_expansion_options_status_idx
    ON cc_expansion_options (status, assessment_id, strategy);

COMMIT;
