# Control Center 0.32 — Incident persistence and operator mutations

Status: source-only implementation slice for the 0.32 Incidents / Status / Reports line. It stays inside the 0.32 release boundary and is prepared on `work/cc032-incidents-operator-service-nr1-20260912` for later runner qualification.

## Implemented persistence boundary

The slice provides PostgreSQL persistence for the validated `internal/incidents` read model:

- migration `0013_incident_read_models` with constrained mirrored query columns and JSONB canonical document storage;
- affected-resource index for bounded resource filtering;
- atomic create/replace of the incident document and its resource index;
- optimistic-concurrency replacement using `ObjectPrecondition` plus `ValidateSuccessor` generation/resource-version semantics;
- fail-closed read verification: mirrored columns must exactly match the validated JSON document;
- bounded newest-first list queries with deterministic keyset pagination and exact filters;
- one-extra-row pagination instead of an unbounded count query.

## Implemented operator mutation preparation

`internal/incidents/mutation.go` prepares side-effect-free, optimistic-concurrency-guarded operator mutations:

- **acknowledge**: only `open -> acknowledged`, actor-bound, bounded optional note, timeline event, generation/resource-version successor validation;
- **resolve**: only `acknowledged -> resolved`, mandatory resolution text, optional bounded evidence references, terminal timeline event and rejection of future-dated evidence;
- **runbook/evidence metadata update**: allowed only before resolution, append-only evidence identities, conflicting evidence rebinding rejected, exact evidence repeats ignored;
- resolved incident metadata is immutable through this path;
- evidence/runbook annotation advances `resource_version` but keeps lifecycle `generation` stable; acknowledge/resolve increment generation;
- every prepared mutation carries the exact `ObjectPrecondition` that persistence revalidates atomically.

Nested slices/pointers are cloned before modification so preparation cannot mutate the caller's current read model.

## Implemented authenticated operator service

`internal/incidents/operator.go` provides the application boundary above the read model and mutation layer.

- actor identity is a server-side argument and is never accepted from a request payload;
- capabilities are explicit and bounded: read, list, acknowledge, resolve and evidence/runbook update;
- authorization is injected through one fail-closed `OperatorAccessPolicy` contract;
- object reads collapse authorization denial to `not found` to avoid an incident-ID oracle;
- list authorization runs before storage access, and every returned object is re-authorized;
- clients cannot choose the next `resource_version`;
- every mutation is bound to the exact current object/scope/revision before commit;
- free-form note/resolution/evidence payloads are excluded from the immutable security Audit record;
- mutation persistence and Audit evidence share one `AtomicMutationCommitter` boundary.

## Implemented RBAC and authenticated HTTP composition

The source now has a concrete bridge to the existing Control Center identity and RBAC model instead of a parallel authorization store.

`internal/incidents/rbac_policy.go`:

- maps incident read/list to existing `resources.read`;
- maps acknowledge/resolve/evidence mutation to existing `core.objects.write`;
- resolves incident scopes through a fail-closed `RBACScopeResolver`;
- requires a global grant for an unscoped list query rather than silently broadening a site grant;
- returns denial or dependency failure when the exact permission/scope decision cannot be proven.

`internal/identity/httpapi/external_routes.go` and `internal/incidents/httpapi/wiring.go`:

- derive actor identity only from the canonical authenticated session principal;
- reuse the mandatory first-login password-change gate;
- expose only the read-only RBAC checker needed by application composition;
- keep scope authorization in `OperatorService`, after authentication but before reads/mutations.

The current runtime composition treats incident `ScopeID` as a site RBAC scope. Global bindings continue to authorize through the canonical RBAC semantics. A future topology-aware resolver may support additional scope kinds, but it must not infer or widen scope when mapping is ambiguous.

## Implemented atomic mutation + canonical Audit persistence

`internal/persistence/postgres/incidents_operator.go` implements the concrete durability boundary.

One PostgreSQL transaction now:

1. locks and reads the current incident;
2. revalidates the exact optimistic-concurrency precondition and successor semantics;
3. validates that Audit evidence is bound to the same incident, scope, status, generation and resource-version transition;
4. replaces the incident document and affected-resource index;
5. acquires the canonical Audit-chain advisory lock;
6. prepares and appends a metadata-only event to `cc_audit_events` using the existing hash-chain format;
7. commits only after both the incident mutation and Audit append succeed.

An Audit failure therefore cannot expose a successful incident mutation. No incident-specific competing audit log is introduced.

## Implemented persistence-owned resource versions

`internal/persistence/postgres/incidents_versions.go` provides the concrete `ResourceVersionGenerator`.

- versions use cryptographic OS randomness;
- values are opaque (`irv-...`) and have no ordering semantics;
- they contain no host, tenant, actor, timestamp, credential or provider information;
- invalid current state, unavailable entropy, cancellation or a collision fail closed.

## Implemented runtime route registration

`cmd/control-center/main.go` now composes the 0.32 incident stack from the existing database and identity boundaries:

- PostgreSQL incident repository;
- canonical RBAC checker + fail-closed incident policy;
- persistence-owned resource-version generator;
- atomic incident/Audit committer;
- authenticated incident HTTP adapter;
- common request correlation, panic recovery, access logging and security-header middleware;
- explicit split routing for `/api/v1/incidents` and `/api/v1/incidents/...` only.

The route prefix does not capture lookalike paths such as `/api/v1/incidents-export`.

## HTTP safety contract

`internal/incidents/httpapi` provides:

- bounded list and exact incident lookup;
- acknowledge, resolve and evidence/runbook annotation routes;
- actor identity only from the authenticated-request resolver;
- server-generated mutation timestamps;
- `application/json` requirement and 64 KiB body cap;
- rejection of unknown fields, trailing values and duplicate JSON keys at any nesting level;
- allowlisted/normalized list query parameters;
- no query parameters on mutation requests;
- bounded error mapping for step-up, conflict, validation, not-found, authorization and dependency-unavailable states;
- `no-store` and `nosniff` JSON responses.

## Security and integrity rules

The implementation fails closed when:

- a stored document is invalid or disagrees with indexed columns;
- an optimistic-concurrency precondition no longer matches;
- object/scope/revision Audit binding differs from the mutation being committed;
- a resource version cannot be generated or does not change;
- an operator attempts an invalid lifecycle transition;
- evidence metadata attempts conflicting rebinding or future collection time;
- a no-op metadata request would only churn resource version;
- identity, RBAC, scope resolution or persistence dependencies are unavailable;
- list policy and storage scope disagree;
- the atomic mutation+Audit transaction fails;
- HTTP actor/media-type/JSON/query validation cannot be proven.

Authentication alone never grants incident authority. The existing RBAC database remains authoritative.

## Migration and rollback

The up migration creates only new 0.32 incident tables/indexes and does not rewrite existing 0.31 data. The down migration removes the new child/index tables before the incident table. Existing Control Center tables remain intact.

## Remaining 0.32 integration work

The following is intentionally still separate and remains within the 0.32 boundary:

- runner qualification and reconciliation of this work branch with the newer `main` head;
- purpose-bound step-up/MFA policy where the authoritative security profile requires it;
- optional narrower persisted incident-specific permissions if approved instead of the current conservative reuse of `resources.read` / `core.objects.write`;
- ingestion/correlation write path and immutable signal-source evidence;
- status-page privacy projection;
- email/webhook notification fan-out;
- technical report/API export and UI presentation.

No work in this slice advances into 0.34 or later.

## Prepared tests

Runner-free test source now covers, among other cases:

- lifecycle mutation/precondition/evidence semantics;
- authorization denial and list scope mismatch;
- server-side actor derivation and mandatory password-change fail-closed behavior;
- RBAC mapping, scope resolution and unsupported capability rejection;
- strict HTTP request validation;
- exact Audit revision binding and unsupported action rejection;
- opaque resource-version generation and entropy failure;
- runtime split routing and incident lookalike-path isolation.

These tests have been written but were not intentionally launched by this non-runner workstream.

## CI / runner boundary

No pull request, workflow dispatch, rerun, check rerun, merge or release action was performed by this source-only slice. Repository Public CI triggers pushes only for `main`, `release/**`, `develop/**` and `migration/**`, while PR workflows require a pull request. This branch is `work/**` and currently has no open PR. Workflow-run inspection after representative commits returned no runs.
