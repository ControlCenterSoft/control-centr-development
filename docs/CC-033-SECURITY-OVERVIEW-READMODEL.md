# Control Center 0.33 — Security / Identity read models

Статус: runner-free source slice, подготовленный поверх canonical development main после 0.32 Audit browser integration. Не является Release Candidate/Public Stable и не меняет текущий PUBLIC STABLE 0.31.0.

## Назначение

0.33 по каноническому Roadmap должен дать полноценный Identity / RBAC / Session / Security Settings UI. Этот source slice объединяет три ранее подготовленные и не вошедшие в canonical main read-only границы без дублирования текущих 0.32 runner-потоков и отдельного NR1 Session Security Workspace.

### `ui.security-overview/v1`

Self-only проекция для текущего аутентифицированного пользователя: identity, first-login/password-change state, bounded password lifecycle, эффективная session policy, активные сессии и effective RBAC grants. Subject определяется только серверным authenticated context; client-selected actor отсутствует. `self_only=true`, `mutation_authorized=false`.

### `ui.permission-explanation/v1`

Bounded объяснение authoritative server-side authorization decision для конкретных permission + scope. Контракт сверяет server decision с уже вычисленными effective grants и fail closed при decision/grant drift. Он не является authorization token, не создаёт access и всегда возвращает `mutation_authorized=false`.

### `ui.identity-access-catalog/v1`

Отдельная global-only административная read-модель пользователей, ролей и bindings. Она не расширяет self-only API параметром другого actor. Каталог строится только при одновременном authoritative global `identity.users.read` + `identity.roles.read`; server decisions повторно сверяются с supplied effective grants. Выход содержит только bounded user metadata, role permissions и bindings. Password hashes, session/token material и другие secrets не проецируются. `global_only=true`, `read_authorized=true`, `mutation_authorized=false`.

## Security boundary

- Self-only subject берётся из authenticated Principal/session и не принимается из query/body.
- Admin catalog допускает только global read authorization; site/tenant/resource-scoped grants не повышаются до global.
- Password/hash, session token/token digest, credential version, raw repository state и secret material отсутствуют в output.
- Password lifecycle содержит только `changed_at`, `change_required|active` и required flag. Timestamp обязателен для Security Overview, не может предшествовать identity creation и не может быть future.
- Current session должна однозначно присутствовать среди active sessions; duplicate, expired, missing или contradictory current marker блокирует projection.
- Session policy обязана иметь валидные absolute/idle TTL semantics; activity не может продлевать absolute expiry.
- Security text проверяется на canonical form, bounds и control characters; Source IP дополнительно обязан быть валидным IP address.
- Effective grants обязаны иметь valid scope, canonical role/permission names и не содержать duplicate grant/permission evidence.
- Permission explanation не вычисляет доступ самостоятельно: server-side decision и supplied grants должны совпадать.
- Admin catalog считается целостным snapshot: duplicate user id/username, duplicate role/permission/binding, unknown user/role reference, invalid scope или impossible/future timestamp блокируют ответ вместо частично «здорового» списка.
- Порядок sessions/grants/permissions/matching roles/users/roles/bindings детерминирован.
- Bounded inputs: Security Overview — до 128 active sessions и 64 effective grants; Admin Catalog — до 256 users, 128 roles, 1024 bindings и 128 permissions на роль.
- `password_change_required` остаётся mandatory boundary и не обходится UI/read-model слоем.
- Текущий Stable 0.31 не подтверждает MFA/WebAuthn/external IdP или отдельный first-admin reset/recovery flow. Эти capability нельзя выводить как доступные только на основании 0.33 read models.

## Согласование с параллельной 0.33 работой

NR1 отдельно подготовил Session Security Workspace и server-side self-only adapter. Этот slice не дублирует его: здесь остаются Security Overview, Permission Explanation и admin-scoped Identity Access Catalog. При последующей runner-интеграции необходимо выбрать единый authenticated adapter/route composition и избежать двух параллельных источников session state.

Текущий 0.32 runner-поток Incidents/Audit/Reports не изменяется и не вытесняется. До завершения/merge актуального 0.32 потока этот 0.33 source остаётся на безопасной `work/**` ветке.

## Handoff для runner-потока

1. Сверить branch с exact current `main` после завершения активной 0.32 integration; при необходимости выполнить чистый transplant/rebase без переноса устаревшего base.
2. Проверить полный compile/test repository и Draft 2020-12 schemas на одном exact SHA.
3. Свести Security Overview с NR1 Session Security Workspace через один authenticated self-only source; actor selector не добавлять.
4. Для Admin Catalog подключить отдельный server-side repository/service boundary, который выдаёт bounded users/roles/bindings snapshot только после global `identity.users.read` + `identity.roles.read`.
5. Добавить HTTP/API negative tests: unauthorized/insufficient scope, first-login/password-change boundary, no-store/security headers, malformed/stale evidence и отсутствие secret fields.
6. Только после exact-head qualification подключать browser UI/accessibility rendering; mutation users/roles/bindings остаётся отдельным slice с собственными permission/Audit/conflict/recovery gates.

## Release / commercial boundary

Этот source slice не меняет SQL migrations, persisted permissions, runtime dependencies, session semantics, authentication methods или commercial/legal obligations. Он не создаёт MFA/external-IdP capability, не даёт mutation/execution/publication authority и не является заявлением о готовности 0.33. Commercial edition/entitlement не может сделать PLANNED или неквалифицированную capability доступной.