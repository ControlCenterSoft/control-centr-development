# Control Center 0.33 — Session Security Workspace handoff

Статус: **SOURCE PREPARED / RUNNER НЕ ЗАПУСКАЛСЯ**.

Рабочая ветка: `work/cc033-session-security-workspace-nr1-b2-20260913`.
База: canonical development `main` `e1854d165f04e72b9d76d4607de5046aadc2a680`.
Source snapshot до добавления этого handoff: `f6f5a7cd4b1b5cf49f331d25121f2eda271b3e1f`.

## Подготовленный scope

- closed `ui.session-security-workspace/v1` schema;
- fail-closed self-only read model текущей identity, effective session policy и активных собственных sessions;
- server-side adapter поверх существующих `SessionSecurityPolicy` и `ListSessions` boundaries;
- authenticated API `GET /api/v1/identity/session-security`;
- browser workspace `GET /web/security/sessions`;
- self-only browser revoke одной/всех sessions через существующие `RevokeSession` / `RevokeAllSessions` service paths и Audit;
- mandatory first-login password-change boundary;
- current-session revoke / revoke-all очищает cookie и требует повторного входа;
- same-origin `Origin` check для browser mutation, `SameSite=Strict`, form-only mutation target и запрет query-selected target;
- отдельный OpenAPI fragment;
- test-only coverage для self-only/credential-free projection, first-login, split routing, same-origin rejection и revoke semantics.

## Что намеренно не заявляется

- тесты не запускались в этой non-runner задаче;
- hosted CI/runner PASS отсутствует;
- ветка не является частью canonical `main` до отдельной qualification/integration;
- VERSION, RC, Stable и publication authority не изменены;
- SQL migration, новая permission, dependency, generic execution, infrastructure mutation и commercial entitlement не добавлены.

## Runner handoff

Перед runner qualification заново сверить фактический `main` и rebase/cherry-pick только если source identity изменилась. Нельзя переносить PASS от другого SHA. Достаточен штатный PR/Public CI для exact resulting head; отдельный synthetic workflow или duplicate rerun для этого slice не требуется. Проверить как минимум:

1. `go test` / existing unit-contract suite для `internal/ui`, `internal/identity/httpapi`, `cmd/control-center`;
2. static/vet/build gates;
3. schema/OpenAPI source checks, если они входят в canonical pipeline;
4. negative first-login / unauthenticated / cross-origin / foreign-session cases;
5. отсутствие token/password/credential leakage;
6. post-merge main qualification только после фактической интеграции.

Этот slice находится в допустимой текущей границе разработки: фактический Public Stable — 0.31.1, поэтому разрешённые будущие trains — 0.32, 0.33 и 0.34. Реализация за 0.34 в этой линии запрещена.