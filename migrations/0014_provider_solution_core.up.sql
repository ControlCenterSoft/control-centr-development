BEGIN;

CREATE TABLE IF NOT EXISTS cc_provider_manifests (
    object_id             varchar(255) PRIMARY KEY,
    provider_id           varchar(128) NOT NULL,
    contract_version      varchar(64)  NOT NULL,
    provider_version      varchar(64)  NOT NULL,
    product_family        varchar(160) NOT NULL,
    qualification_status  text         NOT NULL CHECK (qualification_status IN ('unverified', 'qualified', 'deprecated')),
    manifest_digest       varchar(71)  NOT NULL,
    document              jsonb        NOT NULL,
    created_at            timestamptz  NOT NULL,
    updated_at            timestamptz  NOT NULL,
    CONSTRAINT cc_provider_manifests_object_id_format CHECK (
        object_id ~ '^[A-Za-z0-9](?:[A-Za-z0-9._:-]{0,253}[A-Za-z0-9])?$'
    ),
    CONSTRAINT cc_provider_manifests_provider_id_format CHECK (
        provider_id ~ '^[a-z0-9](?:[a-z0-9._:-]{0,126}[a-z0-9])?$'
    ),
    CONSTRAINT cc_provider_manifests_digest_format CHECK (
        manifest_digest ~ '^sha256:[0-9a-f]{64}$'
    ),
    CONSTRAINT cc_provider_manifests_document_object CHECK (jsonb_typeof(document) = 'object'),
    CONSTRAINT cc_provider_manifests_time_order CHECK (updated_at >= created_at),
    UNIQUE (provider_id, contract_version, provider_version, manifest_digest)
);

CREATE INDEX IF NOT EXISTS cc_provider_manifests_provider_idx
    ON cc_provider_manifests (provider_id, provider_version, qualification_status);

CREATE TABLE IF NOT EXISTS cc_provider_bindings (
    object_id          varchar(255) PRIMARY KEY,
    scope_id           varchar(255) NOT NULL,
    owner_scope        varchar(255) NOT NULL,
    generation         bigint       NOT NULL CHECK (generation > 0),
    resource_version   varchar(255) NOT NULL UNIQUE,
    provider_id        varchar(128) NOT NULL,
    provider_version   varchar(64)  NOT NULL,
    contract_version   varchar(64)  NOT NULL,
    product_family     varchar(160) NOT NULL,
    product_version    varchar(128) NOT NULL,
    management_level   text         NOT NULL CHECK (management_level IN ('observed', 'connected', 'managed')),
    target_ref         varchar(255) NOT NULL,
    health             text         NOT NULL CHECK (health IN ('healthy', 'degraded', 'unavailable', 'unknown')),
    document           jsonb        NOT NULL,
    created_at         timestamptz  NOT NULL,
    updated_at         timestamptz  NOT NULL,
    CONSTRAINT cc_provider_bindings_object_id_format CHECK (
        object_id ~ '^[A-Za-z0-9](?:[A-Za-z0-9._:-]{0,253}[A-Za-z0-9])?$'
    ),
    CONSTRAINT cc_provider_bindings_scope_id_format CHECK (
        scope_id ~ '^[A-Za-z0-9](?:[A-Za-z0-9._:-]{0,253}[A-Za-z0-9])?$'
    ),
    CONSTRAINT cc_provider_bindings_owner_scope_format CHECK (
        owner_scope ~ '^[A-Za-z0-9](?:[A-Za-z0-9._:-]{0,253}[A-Za-z0-9])?$'
    ),
    CONSTRAINT cc_provider_bindings_provider_id_format CHECK (
        provider_id ~ '^[a-z0-9](?:[a-z0-9._:-]{0,126}[a-z0-9])?$'
    ),
    CONSTRAINT cc_provider_bindings_resource_version_format CHECK (
        length(resource_version) BETWEEN 1 AND 255 AND resource_version !~ '[[:space:]]'
    ),
    CONSTRAINT cc_provider_bindings_document_object CHECK (jsonb_typeof(document) = 'object'),
    CONSTRAINT cc_provider_bindings_time_order CHECK (updated_at >= created_at)
);

CREATE INDEX IF NOT EXISTS cc_provider_bindings_scope_idx
    ON cc_provider_bindings (scope_id, provider_id, management_level, health, object_id);

CREATE INDEX IF NOT EXISTS cc_provider_bindings_target_idx
    ON cc_provider_bindings (provider_id, target_ref, object_id);

CREATE TABLE IF NOT EXISTS cc_infrastructure_solutions (
    object_id             varchar(255) PRIMARY KEY,
    scope_id              varchar(255) NOT NULL,
    owner_scope           varchar(255) NOT NULL,
    generation            bigint       NOT NULL CHECK (generation > 0),
    resource_version      varchar(255) NOT NULL UNIQUE,
    display_name          varchar(160) NOT NULL,
    state                 text         NOT NULL CHECK (state IN (
        'draft', 'designed', 'validated', 'planned', 'deploying', 'adopting',
        'verifying', 'ready', 'operating', 'scaling', 'updating', 'reconfiguring',
        'maintaining', 'migrating', 'recovering', 'shrinking', 'decommissioning',
        'degraded', 'failed', 'unknown', 'recovery_required'
    )),
    current_revision_id   varchar(255),
    document              jsonb        NOT NULL,
    created_at            timestamptz  NOT NULL,
    updated_at            timestamptz  NOT NULL,
    CONSTRAINT cc_infrastructure_solutions_object_id_format CHECK (
        object_id ~ '^[A-Za-z0-9](?:[A-Za-z0-9._:-]{0,253}[A-Za-z0-9])?$'
    ),
    CONSTRAINT cc_infrastructure_solutions_scope_id_format CHECK (
        scope_id ~ '^[A-Za-z0-9](?:[A-Za-z0-9._:-]{0,253}[A-Za-z0-9])?$'
    ),
    CONSTRAINT cc_infrastructure_solutions_owner_scope_format CHECK (
        owner_scope ~ '^[A-Za-z0-9](?:[A-Za-z0-9._:-]{0,253}[A-Za-z0-9])?$'
    ),
    CONSTRAINT cc_infrastructure_solutions_resource_version_format CHECK (
        length(resource_version) BETWEEN 1 AND 255 AND resource_version !~ '[[:space:]]'
    ),
    CONSTRAINT cc_infrastructure_solutions_display_name_nonempty CHECK (length(display_name) > 0),
    CONSTRAINT cc_infrastructure_solutions_document_object CHECK (jsonb_typeof(document) = 'object'),
    CONSTRAINT cc_infrastructure_solutions_time_order CHECK (updated_at >= created_at)
);

CREATE INDEX IF NOT EXISTS cc_infrastructure_solutions_scope_state_idx
    ON cc_infrastructure_solutions (scope_id, state, display_name, object_id);

CREATE TABLE IF NOT EXISTS cc_solution_revisions (
    object_id              varchar(255) PRIMARY KEY,
    scope_id               varchar(255) NOT NULL,
    owner_scope            varchar(255) NOT NULL,
    generation             bigint       NOT NULL CHECK (generation > 0),
    resource_version       varchar(255) NOT NULL UNIQUE,
    solution_id            varchar(255) NOT NULL REFERENCES cc_infrastructure_solutions(object_id) ON DELETE CASCADE,
    ordinal                bigint       NOT NULL CHECK (ordinal > 0),
    state                  text         NOT NULL CHECK (state IN ('proposed', 'validated', 'planned', 'active', 'superseded', 'stale')),
    intent_revision_id     varchar(255),
    blueprint_revision_id  varchar(255),
    topology_digest        varchar(71)  NOT NULL,
    document               jsonb        NOT NULL,
    created_at             timestamptz  NOT NULL,
    updated_at             timestamptz  NOT NULL,
    CONSTRAINT cc_solution_revisions_object_id_format CHECK (
        object_id ~ '^[A-Za-z0-9](?:[A-Za-z0-9._:-]{0,253}[A-Za-z0-9])?$'
    ),
    CONSTRAINT cc_solution_revisions_scope_id_format CHECK (
        scope_id ~ '^[A-Za-z0-9](?:[A-Za-z0-9._:-]{0,253}[A-Za-z0-9])?$'
    ),
    CONSTRAINT cc_solution_revisions_owner_scope_format CHECK (
        owner_scope ~ '^[A-Za-z0-9](?:[A-Za-z0-9._:-]{0,253}[A-Za-z0-9])?$'
    ),
    CONSTRAINT cc_solution_revisions_resource_version_format CHECK (
        length(resource_version) BETWEEN 1 AND 255 AND resource_version !~ '[[:space:]]'
    ),
    CONSTRAINT cc_solution_revisions_topology_digest_format CHECK (
        topology_digest ~ '^sha256:[0-9a-f]{64}$'
    ),
    CONSTRAINT cc_solution_revisions_document_object CHECK (jsonb_typeof(document) = 'object'),
    CONSTRAINT cc_solution_revisions_time_order CHECK (updated_at >= created_at),
    UNIQUE (solution_id, ordinal)
);

CREATE INDEX IF NOT EXISTS cc_solution_revisions_solution_state_idx
    ON cc_solution_revisions (solution_id, state, ordinal DESC);

CREATE TABLE IF NOT EXISTS cc_solution_topology_nodes (
    revision_id       varchar(255) NOT NULL REFERENCES cc_solution_revisions(object_id) ON DELETE CASCADE,
    node_id           varchar(255) NOT NULL,
    kind              varchar(128) NOT NULL,
    provider_binding  varchar(255),
    failure_domain    varchar(255),
    document          jsonb        NOT NULL DEFAULT '{}'::jsonb,
    PRIMARY KEY (revision_id, node_id),
    CONSTRAINT cc_solution_topology_nodes_id_nonempty CHECK (length(node_id) > 0),
    CONSTRAINT cc_solution_topology_nodes_kind_nonempty CHECK (length(kind) > 0),
    CONSTRAINT cc_solution_topology_nodes_document_object CHECK (jsonb_typeof(document) = 'object')
);

CREATE INDEX IF NOT EXISTS cc_solution_topology_nodes_provider_idx
    ON cc_solution_topology_nodes (provider_binding, revision_id, node_id)
    WHERE provider_binding IS NOT NULL;

CREATE TABLE IF NOT EXISTS cc_solution_topology_edges (
    revision_id  varchar(255) NOT NULL REFERENCES cc_solution_revisions(object_id) ON DELETE CASCADE,
    from_node    varchar(255) NOT NULL,
    to_node      varchar(255) NOT NULL,
    relation     varchar(128) NOT NULL,
    document     jsonb        NOT NULL DEFAULT '{}'::jsonb,
    PRIMARY KEY (revision_id, from_node, to_node, relation),
    FOREIGN KEY (revision_id, from_node) REFERENCES cc_solution_topology_nodes(revision_id, node_id) ON DELETE CASCADE,
    FOREIGN KEY (revision_id, to_node) REFERENCES cc_solution_topology_nodes(revision_id, node_id) ON DELETE CASCADE,
    CONSTRAINT cc_solution_topology_edges_no_self CHECK (from_node <> to_node),
    CONSTRAINT cc_solution_topology_edges_relation_nonempty CHECK (length(relation) > 0),
    CONSTRAINT cc_solution_topology_edges_document_object CHECK (jsonb_typeof(document) = 'object')
);

CREATE INDEX IF NOT EXISTS cc_solution_topology_edges_relation_idx
    ON cc_solution_topology_edges (revision_id, relation, from_node, to_node);

CREATE TABLE IF NOT EXISTS cc_architecture_validation_results (
    object_id             varchar(255) PRIMARY KEY,
    scope_id              varchar(255) NOT NULL,
    owner_scope           varchar(255) NOT NULL,
    generation            bigint       NOT NULL CHECK (generation > 0),
    resource_version      varchar(255) NOT NULL UNIQUE,
    solution_revision_id  varchar(255) NOT NULL REFERENCES cc_solution_revisions(object_id) ON DELETE CASCADE,
    status                text         NOT NULL CHECK (status IN ('valid', 'warning', 'blocked', 'unknown')),
    evaluated_at          timestamptz  NOT NULL,
    fresh_until           timestamptz  NOT NULL,
    document              jsonb        NOT NULL,
    created_at            timestamptz  NOT NULL,
    updated_at            timestamptz  NOT NULL,
    CONSTRAINT cc_architecture_validation_results_object_id_format CHECK (
        object_id ~ '^[A-Za-z0-9](?:[A-Za-z0-9._:-]{0,253}[A-Za-z0-9])?$'
    ),
    CONSTRAINT cc_architecture_validation_results_scope_id_format CHECK (
        scope_id ~ '^[A-Za-z0-9](?:[A-Za-z0-9._:-]{0,253}[A-Za-z0-9])?$'
    ),
    CONSTRAINT cc_architecture_validation_results_owner_scope_format CHECK (
        owner_scope ~ '^[A-Za-z0-9](?:[A-Za-z0-9._:-]{0,253}[A-Za-z0-9])?$'
    ),
    CONSTRAINT cc_architecture_validation_results_resource_version_format CHECK (
        length(resource_version) BETWEEN 1 AND 255 AND resource_version !~ '[[:space:]]'
    ),
    CONSTRAINT cc_architecture_validation_results_freshness CHECK (fresh_until >= evaluated_at),
    CONSTRAINT cc_architecture_validation_results_document_object CHECK (jsonb_typeof(document) = 'object'),
    CONSTRAINT cc_architecture_validation_results_time_order CHECK (updated_at >= created_at)
);

CREATE INDEX IF NOT EXISTS cc_architecture_validation_results_revision_idx
    ON cc_architecture_validation_results (solution_revision_id, evaluated_at DESC, object_id);

CREATE TABLE IF NOT EXISTS cc_architecture_validation_findings (
    validation_id      varchar(255) NOT NULL REFERENCES cc_architecture_validation_results(object_id) ON DELETE CASCADE,
    ordinal            integer      NOT NULL CHECK (ordinal >= 0),
    code               varchar(128) NOT NULL,
    severity           text         NOT NULL CHECK (severity IN ('info', 'warning', 'blocking')),
    reason             text         NOT NULL,
    remediation        text,
    affected_objects   jsonb        NOT NULL DEFAULT '[]'::jsonb,
    PRIMARY KEY (validation_id, ordinal),
    CONSTRAINT cc_architecture_validation_findings_code_nonempty CHECK (length(code) > 0),
    CONSTRAINT cc_architecture_validation_findings_reason_nonempty CHECK (length(reason) > 0),
    CONSTRAINT cc_architecture_validation_findings_affected_array CHECK (jsonb_typeof(affected_objects) = 'array')
);

CREATE INDEX IF NOT EXISTS cc_architecture_validation_findings_severity_idx
    ON cc_architecture_validation_findings (severity, validation_id, ordinal);

CREATE TABLE IF NOT EXISTS cc_deployment_plans (
    object_id             varchar(255) PRIMARY KEY,
    scope_id              varchar(255) NOT NULL,
    owner_scope           varchar(255) NOT NULL,
    generation            bigint       NOT NULL CHECK (generation > 0),
    resource_version      varchar(255) NOT NULL UNIQUE,
    solution_revision_id  varchar(255) NOT NULL REFERENCES cc_solution_revisions(object_id) ON DELETE CASCADE,
    plan_digest           varchar(71)  NOT NULL,
    state                 text         NOT NULL CHECK (state IN ('draft', 'validated', 'stale', 'superseded')),
    document              jsonb        NOT NULL,
    created_at            timestamptz  NOT NULL,
    updated_at            timestamptz  NOT NULL,
    CONSTRAINT cc_deployment_plans_object_id_format CHECK (
        object_id ~ '^[A-Za-z0-9](?:[A-Za-z0-9._:-]{0,253}[A-Za-z0-9])?$'
    ),
    CONSTRAINT cc_deployment_plans_scope_id_format CHECK (
        scope_id ~ '^[A-Za-z0-9](?:[A-Za-z0-9._:-]{0,253}[A-Za-z0-9])?$'
    ),
    CONSTRAINT cc_deployment_plans_owner_scope_format CHECK (
        owner_scope ~ '^[A-Za-z0-9](?:[A-Za-z0-9._:-]{0,253}[A-Za-z0-9])?$'
    ),
    CONSTRAINT cc_deployment_plans_resource_version_format CHECK (
        length(resource_version) BETWEEN 1 AND 255 AND resource_version !~ '[[:space:]]'
    ),
    CONSTRAINT cc_deployment_plans_digest_format CHECK (plan_digest ~ '^sha256:[0-9a-f]{64}$'),
    CONSTRAINT cc_deployment_plans_document_object CHECK (jsonb_typeof(document) = 'object'),
    CONSTRAINT cc_deployment_plans_time_order CHECK (updated_at >= created_at)
);

CREATE INDEX IF NOT EXISTS cc_deployment_plans_revision_state_idx
    ON cc_deployment_plans (solution_revision_id, state, created_at DESC, object_id);

CREATE TABLE IF NOT EXISTS cc_deployment_steps (
    plan_id      varchar(255) NOT NULL REFERENCES cc_deployment_plans(object_id) ON DELETE CASCADE,
    step_id      varchar(255) NOT NULL,
    ordinal      integer      NOT NULL CHECK (ordinal >= 0),
    capability   varchar(128) NOT NULL,
    target_ref   varchar(255) NOT NULL,
    document     jsonb        NOT NULL DEFAULT '{}'::jsonb,
    PRIMARY KEY (plan_id, step_id),
    UNIQUE (plan_id, ordinal),
    CONSTRAINT cc_deployment_steps_id_nonempty CHECK (length(step_id) > 0),
    CONSTRAINT cc_deployment_steps_capability_nonempty CHECK (length(capability) > 0),
    CONSTRAINT cc_deployment_steps_target_nonempty CHECK (length(target_ref) > 0),
    CONSTRAINT cc_deployment_steps_document_object CHECK (jsonb_typeof(document) = 'object')
);

CREATE TABLE IF NOT EXISTS cc_deployment_step_dependencies (
    plan_id             varchar(255) NOT NULL,
    step_id             varchar(255) NOT NULL,
    depends_on_step_id  varchar(255) NOT NULL,
    PRIMARY KEY (plan_id, step_id, depends_on_step_id),
    FOREIGN KEY (plan_id, step_id) REFERENCES cc_deployment_steps(plan_id, step_id) ON DELETE CASCADE,
    FOREIGN KEY (plan_id, depends_on_step_id) REFERENCES cc_deployment_steps(plan_id, step_id) ON DELETE CASCADE,
    CONSTRAINT cc_deployment_step_dependencies_no_self CHECK (step_id <> depends_on_step_id)
);

CREATE INDEX IF NOT EXISTS cc_deployment_step_dependencies_reverse_idx
    ON cc_deployment_step_dependencies (plan_id, depends_on_step_id, step_id);

COMMIT;
