# Control Center 0.32 — bounded Audit CSV export

Статус: **SOURCE-ONLY / NOT ROUTE-ACTIVE / NOT RELEASE QUALIFIED**.

Этот документ описывает подготовленный исходный slice для milestone 0.32 `Health / Incidents / Audit / Reports`. Он не меняет Public Stable, не открывает новый route в runtime и не является доказательством готовности релиза. Активация выполняется только после отдельной qualification в runner-потоке и интеграционной проверки exact head.

## Назначение

Audit export нужен для ограниченной выгрузки уже доступной оператору части append-only Audit без превращения Control Center в средство массового экспорта чувствительных данных. Экспорт использует те же точные фильтры и opaque cursor, что и существующее чтение `GET /api/v1/audit/events`, и не расширяет область полномочий.

Подготовленный source contract:

- endpoint: `GET /api/v1/audit/events/export`;
- draft permission boundary: существующий global `audit.events.read`;
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

## Source composition

Подготовлены:

- `internal/identity/audit/export.go` — deterministic bounded CSV builder;
- `internal/identity/audit/export_test.go` — negative/privacy/formula/bounds test source;
- `internal/identity/httpapi/audit_export.go` — fail-closed handler source;
- `internal/identity/httpapi/audit_export_test.go` — permission/evidence/disclosure negative-path test source;
- `api/openapi-audit-export-0.32.yaml` — source-only API contract.

Handler намеренно **не зарегистрирован** в `Server.routes()`. Поэтому текущий runtime и Public Stable не получают новый reachable endpoint от этой source-подготовки.

## Что должен выполнить runner-поток до интеграции

Перед route activation и любым release claim требуется отдельная квалификация exact head:

1. форматирование/compile/unit tests для `internal/identity/audit` и `internal/identity/httpapi`;
2. существующие Audit read/integrity/PostgreSQL regression tests;
3. OpenAPI/schema validation;
4. negative paths: unauthenticated, first-login password change required, permission denied, invalid/repeated/unbounded query;
5. подтверждение, что failure записи `audit.events_export` не выдаёт ни одного CSV byte;
6. no-secret/privacy checks, включая top-level и nested IP minimization;
7. CSV/spreadsheet formula-injection regression checks;
8. после успешной qualification — отдельное решение об интеграции route и повторная qualification нового exact head.

## Границы

Этот slice не создаёт bulk export, background export job, scheduled report, upload во внешний сервис, generic file writer или новый permission. Он не меняет Audit retention, не предоставляет mutation authority и не относится к версиям позже 0.33. Любое расширение за эти границы требует отдельного решения в рамках актуального Roadmap.
