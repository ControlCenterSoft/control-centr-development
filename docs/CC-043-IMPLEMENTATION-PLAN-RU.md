# Control Center 0.43 — Implementation Plan

Статус: **EXECUTION PLAN**

Основание: [`CC-043-ARCHITECTURE-FREEZE-RU.md`](CC-043-ARCHITECTURE-FREEZE-RU.md).

Цель 0.43 — реализовать foundation contracts и первый сквозной InfrastructureSolution path без обхода существующих Identity/RBAC, Change/Job, Audit и Recovery boundaries. Production-grade Market providers выпускаются в последующих milestones; reference adapters 0.43 используются для qualification Framework и не должны создавать преждевременные public capability claims.

## 1. Definition of Done 0.43

0.43 считается готовой только если одновременно выполнены:

1. domain contracts и state machines;
2. immutable additive DB migrations поверх 0001–0013;
3. OpenAPI read/write contracts и RBAC;
4. Provider Contract v1 + bounded Provider Runtime;
5. Intent → Blueprint → Synthesis → Validation → SolutionRevision → DeploymentPlan path;
6. все mutations проходят Change/Approval/Job;
7. read-focused Product Web UI для Providers/Solutions/Validation/Readiness;
8. reference qualification Greenfield + Brownfield + expansion + failure paths;
9. supported upgrade from latest Stable line с data/admin-password preservation;
10. no false Success и explicit UNKNOWN/STALE semantics.

## 2. Workstream W0 — Contract baseline

### Domain packages

Создать или нормализовать boundaries:

- `internal/providers`
- `internal/solutions`
- `internal/intent`
- `internal/blueprints`
- `internal/synthesis`
- `internal/ipam`
- `internal/storageinfra`
- `internal/networkfabric`
- `internal/baremetal`
- `internal/time`
- `internal/pki`
- `internal/secrets`
- `internal/artifacts`
- `internal/facilities`
- `internal/licensing`
- `internal/bom`
- `internal/commissioning`
- `internal/operationalpolicy`
- `internal/drift`
- `internal/vulnerability`
- `internal/assetlifecycle`
- `internal/externaldeps`
- `internal/datagovernance`
- `internal/integrations`
- `internal/risk`

### Shared contracts

Каждый versioned resource использует stable ID, scope, generation/resource_version, created_at/updated_at и explicit freshness/evidence where applicable.

Обязательные enums/state machines не должны зависеть от UI strings.

## 3. Workstream W1 — PostgreSQL persistence

Released migrations `0001`–`0013` не изменяются.

Предлагаемая additive schema line 0.43:

- `0014_provider_solution_core.up.sql` — provider manifests/bindings/capabilities, InfrastructureSolution, SolutionRevision, topology/dependency edges, validation results, deployment plans.
- `0015_intent_blueprints_synthesis.up.sql` — Intent revisions, Blueprint revisions, synthesis assessments/candidates/decisions, expansion assessments.
- `0016_infrastructure_foundations.up.sql` — IPAM allocations, network/storage/bare-metal metadata and evidence references.
- `0017_trust_artifact_foundations.up.sql` — time trust metadata, PKI metadata, SecretRef/Credential metadata only, Artifact identity/admission/repository metadata.
- `0018_physical_licensing_bom.up.sql` — facilities/failure domains, third-party entitlement/supportability metadata, BOM requirements/reservations/gaps/readiness.
- `0019_commissioning_operational_policy.up.sql` — commissioning/acceptance/accepted baseline, maintenance/change/freeze windows, disruption/concurrency policies.
- `0020_drift_security_posture.up.sql` — config baselines/snapshots/findings, compliance assessments, vulnerability/exposure/patch posture metadata.
- `0021_asset_external_dependencies.up.sql` — asset lifecycle/warranty/spares/replacement and external dependency/path/health metadata.
- `0022_governance_integrations_risk.up.sql` — data retention/governance metadata, integration bindings/delivery state, cross-domain risk/readiness objects.

Правила:

- большие artifacts/backups/raw telemetry не помещаются в primary DB;
- secret material не хранится в этих tables;
- snapshots bounded/versioned, heavy evidence хранится по reference;
- все migration-файлы после публикации immutable byte-for-byte;
- empty DB, upgrade-from-Stable, restart/idempotency и checksum tests обязательны.

## 4. Workstream W2 — OpenAPI / HTTP API

Разделить contracts, но сохранить общий `/api/v1`.

Предлагаемые спецификации:

- `api/openapi-providers-v1.yaml`
- `api/openapi-solutions-v1.yaml`
- `api/openapi-intent-v1.yaml`
- `api/openapi-infrastructure-foundations-v1.yaml`
- `api/openapi-trust-foundations-v1.yaml`
- `api/openapi-readiness-v1.yaml`
- `api/openapi-commissioning-v1.yaml`
- `api/openapi-operational-policy-v1.yaml`
- `api/openapi-drift-security-v1.yaml`
- `api/openapi-risk-v1.yaml`

### Read endpoints

Минимум:

- `GET /api/v1/providers`
- `GET /api/v1/providers/{id}`
- `GET /api/v1/solutions`
- `GET /api/v1/solutions/{id}`
- `GET /api/v1/solutions/{id}/topology`
- `GET /api/v1/solutions/{id}/validation`
- `GET /api/v1/solutions/{id}/readiness`
- `GET /api/v1/solutions/{id}/risks`
- `GET /api/v1/intents/{id}`
- `GET /api/v1/synthesis/{id}`
- `GET /api/v1/deployment-plans/{id}`
- bounded foundation read endpoints for IPAM/artifacts/PKI/secrets metadata/physical/licensing/BOM/commissioning/drift/security posture.

### Mutation endpoints

HTTP mutation не выполняет provider action inline. Она создаёт revision/assessment/Change/Job:

- create/update Intent revision;
- start synthesis assessment;
- select VALID candidate;
- create Proposed SolutionRevision;
- validate architecture;
- create DeploymentPlan;
- request adoption/reconciliation/expansion;
- approve/execute only through existing Change/Job boundary.

ETag/If-Match or explicit expected revision mandatory. Dynamic target expansion after approval forbidden.

### RBAC

Минимум:

- `providers.read`, `provider_bindings.read`, `provider_bindings.manage`
- `solutions.read`, `solutions.create`, `solutions.edit`, `solutions.validate`, `solutions.plan`, `solutions.reconcile`
- `blueprints.read`, `blueprints.manage`
- `intent.read`, `intent.manage`
- scoped read/manage permissions for foundation domains;
- no human-facing generic `provider.execute` permission.

## 5. Workstream W3 — Provider Contract v1 / Runtime

### Manifest

Provider manifest pins:

- provider ID/version/contract version;
- product family + supported product versions;
- management levels OBSERVED/CONNECTED/MANAGED;
- capabilities;
- required permissions/SecretRefs;
- network/storage/capacity requirements;
- risk class;
- preconditions/locks/idempotency;
- verification/failure/rollback/recovery model;
- qualification status and integration contracts.

### Runtime

`API → Change/Job Engine → Provider Runtime → Provider Adapter → Product`.

Adapter получает только typed capability, exact targets, scoped SecretRefs, execution context, timeout and idempotency key. Unrestricted root/shell contract запрещён.

### 0.43 reference adapters

Создать qualification adapters/profiles достаточные для проверки Framework:

- Proxmox VE reference adapter;
- PBS integration reference adapter;
- Zabbix observation/integration reference adapter;
- Samba AD/DNS observation/reference adapter;
- Redfish bare-metal adapter;
- generic/mock managed-switch contract adapter.

Эти adapters в 0.43 не означают публичную production-ready Market capability milestone 0.44/0.51/0.52; public claim появляется только после собственного provider qualification/release.

## 6. Workstream W4 — Solution Orchestrator

Реализовать вертикальный path:

`IntentRevision → BlueprintRevision → Current Infrastructure Snapshot → Candidate Synthesis → Hard Constraint Filter → Scoring → Architecture Validation → Decision → Proposed SolutionRevision → DeploymentPlan DAG`.

### Required candidate dimensions

- compute/capacity + forecast;
- network/storage;
- site/failure domains/physical/power;
- recovery RPO/RTO;
- security/trust;
- licensing/supportability;
- artifacts/offline readiness;
- BOM/material gap;
- operational complexity/cost evidence where known.

UNKNOWN hard evidence blocks candidate. Operator may select nonrecommended VALID candidate with Audit evidence. BLOCKED candidate cannot execute.

### Expansion

Support strategies: SCALE_UP, SCALE_OUT, NEW_INSTANCE, NEW_CLUSTER, SPLIT_MIGRATE, REPLACE, NO_CHANGE. Expansion always creates new SolutionRevision.

## 7. Workstream W5 — Foundation services

Implement as metadata/control-plane first, execution only through typed providers.

Priority order:

1. IPAM/Naming + reservations;
2. Artifact admission/repository metadata;
3. Time trust evidence;
4. PKI metadata/rotation plans;
5. Secrets/service identity metadata/grants;
6. Physical/failure-domain model;
7. Third-party licensing/supportability;
8. BOM/readiness;
9. Commissioning/acceptance;
10. Operational policy/change eligibility;
11. Config baseline/drift/compliance;
12. Vulnerability/exposure/patch posture;
13. Asset lifecycle/spares;
14. External dependencies;
15. Data governance/retention;
16. Integrations;
17. cross-domain risk/readiness.

Foundation services MUST NOT directly mutate infrastructure outside the established Change/Job/provider path.

## 8. Workstream W6 — Product Web UI

0.43 UI is operator-focused and read-first.

### Navigation

- `Infrastructure → Solutions`
- `Infrastructure → Providers`
- `Infrastructure → Readiness`
- `Infrastructure → Addressing/IPAM`
- `Infrastructure → Physical`
- `Security → Trust & Certificates`
- `Security → Secrets & Identities`
- `Security → Posture`
- `Operations → Commissioning`
- `Operations → Policies`
- `Operations → Drift`

### Core views

**Solutions list** — lifecycle, health, readiness, provider count, current revision, stale/degraded markers.

**Solution detail** — Intent summary, topology/dependency graph, Desired/Actual, validation findings, BOM/readiness, deployment/expansion plans, commissioning, risks.

**Provider detail** — management level, supported versions/capabilities, bindings, health, qualification status. Never imply unsupported capability.

**Candidate comparison** — 2–5 candidates with hard constraints, trade-offs, reuse, capacity horizon, licensing delta, BOM delta, risk reasons.

**Action Center integration** — blockers and exact recommended next action, not raw warning counts.

Status must never rely on color only. Unknown/stale states explicitly visible.

## 9. Workstream W7 — Tests / qualification

### Contract tests

- schema validation;
- provider capability matrix;
- unsupported capability fail-closed;
- stale revisions and ETag/preconditions;
- cross-scope RBAC denial;
- secret/reference redaction;
- dynamic target expansion prevention.

### Persistence tests

- clean install;
- 0.31/0.32 compatible upgrade path according to release matrix;
- migrations 0001–0013 checksum immutability;
- additive 0014–0022 application;
- restart/idempotency;
- admin password/user data/Change/Job preservation.

### Reference qualification scenarios

- Greenfield empty hardware → managed solution;
- Brownfield discovery/adoption without forced reinstall;
- add node / create second cluster expansion;
- network/storage/power failure-domain validation;
- artifact mismatch/quarantine;
- credential/PKI/time failures;
- licensing UNKNOWN/shortfall;
- BOM gap and reservation conflict;
- commissioning failure and accepted baseline;
- maintenance/disruption budget conflict;
- drift detection and safe reconciliation proposal;
- vulnerability + mitigation/patch plan;
- external dependency failure;
- integration outage/replay/conflict;
- no false Success.

## 10. Workstream W8 — Release slices

0.43 should be integrated in bounded slices rather than one giant PR:

- **Slice A:** contracts + 0014/0015 + read-only Provider/Solution/Intent APIs;
- **Slice B:** Provider Runtime + reference adapters + validation;
- **Slice C:** IPAM/artifact/trust foundation metadata + 0016/0017;
- **Slice D:** physical/licensing/BOM/readiness + 0018;
- **Slice E:** commissioning + operational policy + 0019;
- **Slice F:** drift/security posture + 0020;
- **Slice G:** lifecycle/external deps + 0021;
- **Slice H:** governance/integrations/risk + 0022;
- **Slice I:** Solution Orchestrator end-to-end + Product Web UI;
- **Slice J:** reference qualification, upgrade/recovery/security gates and 0.43 release candidate.

Каждый slice имеет собственные positive/failure/security tests и не получает статус complete без exact-head CI PASS.

## 11. Architecture Freeze exit

После начала Slice A фундаментальный scope 0.43 заморожен. Новая foundation capability требует отдельного roadmap change; обычные clarifications/bugs/hardening выполняются внутри зафиксированных domains.
