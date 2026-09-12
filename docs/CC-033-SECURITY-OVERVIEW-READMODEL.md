# Control Center 0.33 — Security Overview read model

Статус: runner-free source slice для последующей интеграции и qualification. Не является доказательством готовности 0.33 и не меняет PUBLIC STABLE.

## Назначение

Контракт `ui.security-overview/v1` формирует безопасное read-only представление для первого экрана будущего milestone 0.33 «Identity / RBAC / Session / Security Settings UI». Он объединяет только уже существующее evidence текущего аутентифицированного пользователя: identity, обязательность смены первоначального пароля, эффективную session policy, активные сессии и effective RBAC grants.

Дополнительный контракт `ui.permission-explanation/v1` формирует bounded объяснение authoritative authorization decision для конкретных permission + scope. Он использует только уже полученные self-only effective grants, проверяет согласованность с server-side `Allowed` и fail closed при decision/grant drift. Это объяснение не является authorization token и всегда возвращает `mutation_authorized=false`.

Контракт `ui.identity-access-catalog/v1` добавляет отдельную **global-only административную read-модель** для вкладок пользователей/ролей/scopes будущего 0.33 UI. Это не расширение self-only endpoint: каталог строится только когда authoritative server-side decisions одновременно разрешают `identity.users.read` и `identity.roles.read` на global scope, а решения совпадают с supplied effective grants. Каталог содержит только безопасные user metadata, role permissions и bindings; password hashes, session/token material и иные secrets в output отсутствуют.

Все три контракта являются только проекциями existing authoritative evidence и не выдают authorization/execution authority. Self-only contracts не получают actor selector, а административный каталог имеет `global_only=true`, `read_authorized=true`, `mutation_authorized=false` и не предоставляет mutation API.

## Security boundary

- Subject self-only представлений задаётся серверным authenticated context, а не пользовательским `subject_id`.
- В projection не попадают password/hash, session token/token digest, credential version, raw repository state или иные секреты.
- Current session должна однозначно присутствовать в active-session evidence; противоречивый marker, expired session или дубликат fail closed.
- Session policy должна иметь положительные bounded TTL/idle semantics; activity не может продлевать absolute expiry.
- RBAC grants принимаются только с валидным scope, canonical role/permission names и без duplicate grant/permission evidence.
- Permission explanation не вычисляет новый доступ: server decision и effective grants обязаны совпадать; иначе ответ не строится.
- Deny explanation различает только безопасные bounded причины: отсутствие grants, отсутствие подходящего scope или отсутствие permission. Внутренние policy/repository details наружу не выдаются.
- Administrative identity catalog требует одновременно global `identity.users.read` + `identity.roles.read`. Site/tenant/resource-scoped grants не повышаются до global доступа.
- Решения `UsersReadAllowed`/`RolesReadAllowed` сверяются с exact supplied effective grants. Любой decision/grant drift блокирует projection fail closed.
- Каталог считается целостным snapshot: duplicate user id/username, duplicate role/permission/binding, binding на отсутствующего user/role, invalid scope или неконсистентный timestamp блокируют ответ вместо частичного «здорового» списка.
- Каталог не проецирует `PasswordHash`. Из password lifecycle разрешены только boolean `password_change_required` и безопасные timestamps; это metadata, а не credential evidence.
- Порядок sessions/grants/permissions/matching roles/users/roles/bindings детерминирован, чтобы UI не зависел от порядка backend storage.
- Bounded input для self overview: максимум 128 active sessions и 64 effective grants; user-agent ограничен 512 байт. Для admin catalog: максимум 256 users, 128 roles, 1024 bindings и 128 permissions на роль.
- `password_change_required` отражается как attention/state и не обходится read-model слоем.

## Что ещё требуется до интеграции 0.33

1. Подключить self-only builders к authenticated self-only HTTP adapter после объединения 0.32 integration work; не принимать actor/subject из query/body.
2. Получать sessions через существующий self session inventory и grants через `rbac.Introspector.EffectiveGrants` для текущего principal.
3. Для admin catalog добавить отдельный authoritative repository/service boundary, который выдаёт полный bounded snapshot users/roles/bindings только после server-side global `PermissionUsersRead` + `PermissionRolesRead`; не переиспользовать self-only endpoint через параметр другого actor.
4. Сохранять существующую fail-closed Audit boundary: если обязательное security read evidence/Audit недоступно, не отдавать частично «здоровый» security overview/catalog.
5. Добавить exact-head HTTP/API tests, schema validation и accessibility/UI rendering tests для overview, permission explanation и identity-access catalog.
6. Mutation slices users/roles/bindings проектировать отдельно с собственными permissions, exact target/revision, Audit и conflict/recovery semantics; `ui.identity-access-catalog/v1` никогда не становится mutation authority.

## Release boundary

Этот slice не меняет SQL migrations, persisted permissions, runtime dependencies, session semantics или commercial/legal obligations. Он не влияет на текущий 0.31 release candidate path и должен квалифицироваться отдельным runner-потоком только после безопасной интеграции в актуальный 0.33 head. До такой qualification каталог остаётся source-only и не является заявлением о готовности 0.33.
