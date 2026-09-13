# Control Center 0.32.0 — Health / Incidents / Audit / Reports

Статус: **PREPARED / НЕ RELEASE CANDIDATE / НЕ PUBLIC STABLE**.

Control Center 0.32.0 — следующий release train после официального Public Stable 0.31.0. Документ фиксирует целевую release boundary и не является заявлением о завершённой qualification.

## Пользовательский результат

0.32 делает эксплуатационное состояние first-class частью Web UI и API:

- Health показывает фактическое состояние ресурсов и freshness evidence;
- Incidents дают bounded операторское представление инцидентов, затронутых ресурсов, сигналов, runbook/evidence references и timeline;
- Audit предоставляет bounded поиск и безопасный CSV export;
- Reports объединяют подтверждённое Health/Incident/Audit evidence и дают resource-bound Evidence Drawer.

Unknown, stale, expired, unavailable, malformed или противоречивое evidence не отображается как Healthy/Success.

## Security boundary

0.32 не вводит generic shell/command API и не выдаёт mutation authority read-only UI слоям.

Обязательные свойства:

- actor берётся только из server-authenticated session;
- global resource и Audit evidence имеют раздельные RBAC boundaries;
- first-login password-change gate сохраняется перед обычной работой;
- API/Web routes не позволяют клиенту выбирать backend provider/source/permission/actor;
- Incident mutation сохраняет optimistic concurrency, authorization и canonical Audit evidence;
- Audit operator view не раскрывает SourceIP, raw Details, credentials, secrets или provider-private payloads;
- CSV export остаётся bounded и нейтрализует spreadsheet-formula injection;
- browser responses используют `no-store`, `nosniff`, frame-deny, no-referrer и same-origin CSP;
- Reports/Evidence Drawer не допускают cross-resource evidence leakage;
- запись события, provider acceptance или наличие Incident сами по себе не доказывают Healthy/Success.

## Persistent state и upgrade

0.32 добавляет только additive schema для собственного durable state. Все migration-файлы, опубликованные в Public Stable 0.31.0, являются immutable byte-for-byte.

Поддерживаемый release path обязан доказать:

- clean install 0.32 exact candidate;
- upgrade с официального 0.31.0;
- сохранение установленного пользователем admin password и first-login state;
- сохранение существующих Change / Job / timeline и другого опубликованного durable state;
- применение 0.32 migration ровно один раз и replay-idempotency;
- восстановление pre-upgrade state;
- повторное forward recovery тем же exact candidate;
- PostgreSQL restart/reconnect на поддерживаемых версиях.

## Packaging

Для exact 0.32 candidate подготавливаются отдельные, не наследуемые от 0.31 release artifacts:

- Linux amd64 archive и SHA-256 sidecar;
- source archive;
- CycloneDX SBOM;
- THIRD_PARTY_NOTICES;
- qualification evidence;
- provenance;
- release manifest;
- `SHA256SUMS`.

Dependency/license inventory привязывается к версии 0.32 отдельно, даже если набор runtime dependencies не изменился относительно 0.31.

## Текущая development boundary

Canonical main уже содержит значительную часть milestone 0.32, включая Incident core, Audit export/browser, Health source model, Reports/Evidence Drawer и report adapters/routes. Однако наличие кода в main или подготовленной ветке не является RC evidence.

Перед формированием точного кандидата должны быть закрыты оставшиеся непокрытые UI/runtime boundaries и выполнена единая exact-SHA qualification всего состава 0.32. Открытые PR и отдельные source-only ветки не включаются в release claim до их qualification/integration.

## Product Stable и commercial launch

Product Public Stable определяется технической готовностью: scope integration, packaging, clean install, supported upgrade, rollback/forward recovery, PostgreSQL restart/reconnect, security/privacy и release metadata.

Commercial/legal clearance ведётся отдельным track. Пока он не закрыт, продукт и публичные материалы не должны заявлять неподтверждённые договорные, лицензионные, support или иные юридические гарантии. Это не преобразует отсутствующее legal evidence в PASS.

## Release stop conditions

0.32.0 не может быть объявлен Release Candidate или Public Stable при любом из следующих условий:

- false Success или улучшение stale/unknown evidence до Healthy;
- нарушение RBAC/first-login/session boundary;
- cross-resource evidence leakage;
- migration drift относительно опубликованного 0.31;
- upgrade теряет данные, настройки или пользовательский пароль;
- rollback/forward recovery не подтверждён;
- exact candidate SHA не прошёл обязательные technical gates;
- release artifacts/checksums/provenance не относятся к одной release identity;
- high-risk security/privacy/recovery defect.

После закрытия этих условий status этого документа должен быть изменён только вместе с exact candidate qualification и официальным release/promotion процессом.
