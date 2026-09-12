# Control Center 0.32 — bounded Audit CSV export

Статус: **0.32 DEVELOPMENT — ROUTE ACTIVATION SLICE / NOT PUBLIC STABLE**.

Этот документ описывает bounded Audit CSV export для milestone 0.32 `Health / Incidents / Audit / Reports`. Source builder/handler уже прошли отдельную qualification; текущий activation slice подключает endpoint к cumulative Identity/RBAC server и production identity wiring только после нового exact-head Public CI. Само наличие route в development `main` не является заявлением о готовности 0.32 к Public Stable.

## Назначение

Audit export нужен для ограниченной выгрузки уже доступной оператору части append-only Audit без превращения Control Center в средство массового экспорта чувствительных данных. Экспорт использует те же точные фильтры и opaque cursor, что и существующее чтение `GET /api/v1/audit/events`, и не расширяет область полномочий.

Контракт:

- endpoint: `GET /api/v1/audit/events/export`;
- permission boundary: существующий global `audit.events.read`;
- first-login password change должен быть завершён до доступа;
- `limit` — от 1 до 100, default 50;
- один запрос экспортирует только одну bounded page, а не весь журнал;
- фильтры `action`, `outcome`, `actor_id`, `subject_id` — только exact match;
- cursor остаётся opaque и не создаётся клиентом самостоятельно.

## Privacy и защита данных

CSV намеренно минимизирован относительно полного Audit event contract:

- отдельная колонка `source_ip` отсутствует;
- вложенные поля деталей, идентифицированные как source/remote/client IP, дополнительно редактируются для экспортного контура;
- действующая secret redaction применяется повторно перед сериализацией details;
- bearer/token/password/credential-like данные не должны попадать в выгрузку;
- текстовые значения с префиксами `=`, `+`, `-`, `@` нейтрализуются перед открытием CSV в spreadsheet-клиенте;
- `Cache-Control: no-store` и `X-Content-Type-Options: nosniff` обязательны.

Hash-chain evidence (`previous_hash`, `hash`) сохраняется в CSV, чтобы выгрузка не теряла связь с фактическим append-only Audit evidence. Экспорт сам по себе не пересчитывает и не переписывает Audit history.

## Fail-closed evidence rule

Успешная выгрузка обязана сначала сформировать `audit.events_export` с результатом `success`, включая bounded query metadata, количество строк, sequence bounds и privacy-policy markers. Только после успешного сохранения этого evidence разрешается отправлять CSV bytes клиенту.

Если журнал недоступен, построение export не прошло validation либо Audit evidence нельзя записать, ответ — `503`; CSV не выдаётся. Это исключает privileged read/export без собственной аудируемой записи.

## Runtime composition

Компоненты:

- `internal/identity/audit/export.go` — deterministic bounded CSV builder;
- `internal/identity/audit/export_test.go` — negative/privacy/formula/bounds coverage;
- `internal/identity/httpapi/audit_export.go` — fail-closed handler;
- `internal/identity/httpapi/audit_export_route.go` — cumulative server route activation;
- `internal/identity/httpapi/audit_export_test.go` — reachable-route permission/evidence/disclosure negative-path coverage;
- `api/openapi-audit-export-0.32.yaml` — API contract;
- `cmd/control-center/identity.go` — production identity server uses the cumulative constructor with Audit export enabled.

Route activation не создаёт нового permission и не ослабляет existing `Authenticate → password-current → audit.events.read/global` boundary.

## Qualification boundary

Перед интеграцией activation slice exact head обязан пройти штатный Public CI без переноса PASS со source-only head:

1. formatting/vet/build/unit-contract;
2. PostgreSQL 15–18 clean-install/supported-upgrade/adapters и restart/race qualification;
3. существующие Audit read/integrity regressions;
4. reachable-route negative paths: unauthenticated, first-login password change required, permission denied, invalid/repeated/unbounded query;
5. подтверждение, что failure записи `audit.events_export` не выдаёт CSV bytes;
6. no-secret/privacy checks, включая IP minimization;
7. CSV/spreadsheet formula-injection checks;
8. exact post-merge `main` qualification после интеграции.

## Границы

Этот slice не создаёт bulk export, background export job, scheduled report, upload во внешний сервис, generic file writer или новый permission. Он не меняет Audit retention, не предоставляет mutation authority, не меняет Public Stable 0.31.0 и не является самостоятельным release claim. Любое расширение за эти границы требует отдельного решения в рамках актуального Roadmap.
