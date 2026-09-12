# Control Center 0.32 — Reports / Evidence boundary hardening

Status: **runner-free source preparation / non-production**.

## Purpose

This NR2 slice extends the existing `ui.operational-report/v1` and `ui.evidence-drawer/v1` source package without duplicating the current Incident or Audit-export runner flows. It closes a transport/storage trust gap: a report built in-memory is canonical, but a report reconstructed from JSON, persistence, IPC or another component must not be trusted merely because its schema string and `mutation_authorized=false` are present.

## Added boundary

`ValidateOperationalReport` performs fail-closed revalidation before a transported/stored report is consumed:

- exact schema and supported `data_state` / health-state validation;
- `mutation_authorized` must remain hard-false;
- non-zero `generated_at` and bounded evidence count;
- report-level `evidence_digest` must be a canonical lower-case SHA-256 and match the canonical evidence projection;
- unavailable and loaded-empty reports must remain `unknown`, never Healthy;
- every evidence item is rebuilt through the existing bounded validator;
- duplicate evidence IDs and duplicate refs are rejected;
- future observations relative to `generated_at` are rejected;
- `effective_state` must equal the state derived from `observed_state + freshness`, so stale/expired evidence cannot be forged back to Healthy;
- evidence refs and report evidence ordering must remain canonical/deterministic;
- `overall_state` is recomputed and must match;
- transport/storage tampering fails closed even if an attacker recomputes only the outer digest after forging semantic fields.

`BuildValidatedEvidenceDrawer` is the consumer boundary for reports crossing JSON/persistence/IPC boundaries. The SHA-256 digest is an integrity binding for canonical representation; it is **not** a signature and does not establish source authenticity or authorization.

## Local qualification performed without GitHub runner

The new validator and negative tests were compiled together with the existing report builder contract in an isolated local Go module. `gofmt` completed and `go test ./internal/ui` passed. Covered cases include canonical builder output, stale digest tamper, forged Healthy effective state, duplicate identity, future observation, non-canonical evidence refs/order, forged overall state, unavailable-with-evidence and Evidence Drawer rejection of a tampered transported report.

## Integration boundary

No runtime route, permission, SQL migration, dependency, Stable identity or publication authority is added. The current runner flow for PR #194 is intentionally untouched. Before 0.32 integration, a runner-qualified exact-current-main transplant/rebase must combine this source package with the qualified Incident/Audit state and then exercise full repository, schema, HTTP/RBAC and UI/accessibility gates.
