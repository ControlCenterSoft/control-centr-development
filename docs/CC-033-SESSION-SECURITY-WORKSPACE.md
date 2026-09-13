# Control Center 0.33 — Session Security Workspace

## Назначение

`Session Security Workspace` — self-only read model и browser workspace для административного интерфейса Control Center 0.33. Он объединяет уже существующие серверные данные текущей identity, effective session policy и активных sessions в один bounded UI-контракт.

Контракт не создаёт новую модель аутентификации и не добавляет отдельный источник полномочий. Источник истины остаётся серверный authentication/session subsystem.

## Граница безопасности

- actor определяется только из аутентифицированной server-side session; client-selected `actor_id` отсутствует;
- workspace недоступен до обязательной смены bootstrap-пароля;
- bearer token, token digest, password/hash, role bindings и credential material в read model отсутствуют;
- `self_only=true` является обязательным свойством контракта;
- `mutation_authorized=false` всегда: наличие кнопки отзыва сессии не является authorization evidence;
- фактический revoke заново проходит существующий authenticated self-only revocation path и Audit;
- target `session_id` не меняет actor scope: сервер всегда передаёт в revocation service только `principal.Identity.ID`, полученный из текущей аутентифицированной сессии;
- browser revoke требует POST, exact same-origin `Origin`, form-urlencoded body и не принимает mutation target из query string; это дополняет `SameSite=Strict` session cookie и не заменяет server-side owner revalidation;
- текущая session должна быть ровно одна и совпадать с authenticated session identity; mismatch блокируется fail-closed;
- expired, duplicate, future-dated или temporal-inconsistent session evidence отклоняется целиком;
- session deadlines обязаны быть согласованы с текущей effective policy: absolute lifetime не может превышать `absolute_ttl_seconds`, а idle deadline — `idle_timeout_seconds` от последней подтверждённой активности;
- непустой `source_ip` обязан быть канонически распознаваемым IP-адресом, а не произвольной строкой;
- unavailable source не может возвращать частичную identity/session проекцию как успешную;
- API и browser responses используют `Cache-Control: no-store`; общая identity boundary сохраняет `nosniff`, `DENY` frame policy, `no-referrer` и restrictive CSP.

## Пользовательское представление

Для каждой активной session допускается показывать:

- session ID как bounded техническую identity;
- время создания;
- последнее подтверждённое activity;
- idle deadline;
- absolute expiry;
- собственный source IP и User-Agent;
- признак текущей session;
- оставшееся время до idle/absolute expiry;
- понятный риск revoke: обычный отзыв либо немедленный logout текущей session.

Effective policy показывает absolute TTL, idle timeout и семантику обновления idle deadline. Activity не может продлевать absolute expiry.

## Fail-closed semantics

Workspace не должен собираться, если:

- отсутствует trusted current time;
- identity/current-session binding неполон;
- password-change-required ещё активен;
- policy выходит за допустимые bounds;
- session deadline противоречит effective policy;
- нет ровно одной current session;
- current marker не соответствует authenticated session;
- обнаружены duplicate session IDs;
- session уже истекла;
- timestamps идут назад или находятся в будущем;
- `source_ip` не является корректным IP-адресом;
- bounded text содержит control characters или превышает лимит.

## Runtime/API wiring

Добавлен cumulative identity-server constructor `NewServerWithSessionSecurityWorkspace`, который сохраняет уже квалифицированные Audit routes и дополнительно регистрирует:

- `GET /api/v1/identity/session-security` — authenticated + mandatory-current-password JSON workspace;
- `GET /web/security/sessions` — browser workspace с redirect на `/login` при отсутствии/истечении session и на `/password/change`, если bootstrap password ещё не заменён;
- `POST /web/security/sessions/revoke` — self-only revoke одной session по bounded `session_id` внутри уже аутентифицированного actor scope;
- `POST /web/security/sessions/revoke-all` — self-only revoke всех sessions текущего actor.

Runtime `newIdentityHandler` использует этот cumulative constructor. API и browser routes переиспользуют существующие service boundaries:

- `SessionSecurityPolicy` / `GET /api/v1/auth/session-policy` semantics;
- `ListSessions` / `GET /api/v1/auth/sessions` semantics;
- `RevokeSession` / `DELETE /api/v1/auth/sessions/{sessionID}` semantics;
- `RevokeAllSessions` / `POST /api/v1/auth/sessions/revoke-all` semantics.

Browser route не принимает subject/actor из query/form. Query `actor_id` не участвует в server-side identity binding. Current-session revoke и revoke-all очищают session cookie и возвращают пользователя на `/login`; отзыв другой собственной session оставляет текущую session активной и возвращает workspace.

## Qualification boundary

Добавлен test-only код для последующей runner qualification: anonymous API denial, self-only/credential-free projection, first-login blocking, cross-origin/query-selected browser mutation denial, отзыв другой собственной session без потери текущей session и current-session revoke с очисткой cookie. В этой non-runner линии тесты не запускались и PASS не заявляется.

Этот slice не добавляет SQL migration, dependency, новую permission, generic execution, infrastructure mutation, external publication или commercial entitlement. Он остаётся в разрешённой границе 0.33 и не изменяет VERSION/RC/Public Stable identity.