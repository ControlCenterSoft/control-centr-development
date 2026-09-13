# Control Center 0.32 — Incidents operator Web UI

Status: bounded read-only UI qualification slice for the committed 0.32 `Health / Incidents / Audit / Reports` release train.

## Scope

This slice exposes the already-qualified Incident read model to an authenticated operator browser surface without creating a parallel authorization or mutation path.

- `GET /api/v1/incidents/ui` renders a bounded incident list using the same exact `ListQuery` semantics as the JSON API.
- `GET /api/v1/incidents/ui/{incidentID}` renders the exact authorized incident with affected resources, signals, runbook reference, incident evidence references and timeline.
- pagination reuses the canonical keyset cursor (`before_started_at` + `before_object_id`) and preserves the exact bounded filters;
- malformed query input is rejected before the operator service is called;
- malformed persisted Incident evidence fails closed with `503` rather than being hidden or rendered as a healthy/empty result;
- the existing JSON API remains unchanged.

## Authorization and security boundary

The browser surface is activated through the same cumulative `NewAuthenticated` incident wiring as the API. Canonical session authentication and the mandatory first-login password-change gate execute before the incident handler. Actor identity comes only from the authenticated server-side principal. `OperatorService` remains authoritative for scoped Incident RBAC; the browser cannot supply an actor or bypass scope policy.

The UI is deliberately read-only in this slice. It does not expose acknowledge, resolve, evidence-update, generic command or production-mutation controls. Existing typed mutation APIs retain their own precondition, step-up, RBAC, Audit and atomic persistence boundaries.

Rendered data is limited to the bounded Incident contract. Provider payloads, credentials, secrets and arbitrary infrastructure state are not introduced. The page sends `Cache-Control: no-store`, `nosniff`, frame denial, no-referrer and the existing same-origin CSP profile.

## Release boundary

This work closes an independent 0.32 operator-visibility gate after the Incident API/persistence/RBAC qualification and does not claim 0.32 Release Candidate or Public Stable by itself. It adds no SQL migration, runtime dependency, permission, execution authority or commercial redistribution obligation.

One exact-head Public CI pass is required. Do not use `workflow_dispatch`, synthetic load, duplicate checks or unchanged reruns. Existing Public safety, format/vet, unit/contracts, build, PostgreSQL 15–18 clean-install/supported-upgrade/adapters and race/restart gates must remain green.
