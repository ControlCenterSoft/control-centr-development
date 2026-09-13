# Control Center 0.33 — Session Security Workspace

## Назначение

`Session Security Workspace` — self-only read model для административного интерфейса Control Center 0.33. Он объединяет уже существующие серверные данные текущей identity, effective session policy и активных sessions в один bounded UI-контракт.

Контракт не создаёт новую модель аутентификации и не добавляет отдельный источник полномочий. Источник истины остаётся серверный authentication/session subsystem.

## Граница безопасности

- actor определяется только из аутентифицированной server-side session; client-selected `actor_id` отсутствует;
- workspace недоступен до обязательной смены bootstrap-пароля;
- bearer token, token digest, password/hash, role bindings и credential material в read model отсутствуют;
- `self_only=true` является обязательным свойством контракта;
- `mutation_authorized=false` всегда: наличие кнопки отзыва сессии не является authorization evidence;
- фактический revoke обязан заново пройти существующий authenticated self-only revocation path и Audit;
- текущая session должна быть ровно одна и совпадать с authenticated session identity; mismatch блокируется fail-closed;
- expired, duplicate, future-dated или temporal-inconsistent session evidence отклоняется целиком;
- unavailable source не может возвращать частичную identity/session проекцию как успешную.

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
- нет ровно одной current session;
- current marker не соответствует authenticated session;
- обнаружены duplicate session IDs;
- session уже истекла;
- timestamps идут назад или находятся в будущем;
- bounded text содержит control characters или превышает лимит.

## Runtime wiring — следующий слой

Runtime/UI wiring должен переиспользовать существующие self-only API/service boundaries:

- `GET /api/v1/auth/session-policy`;
- `GET /api/v1/auth/sessions`;
- `DELETE /api/v1/auth/sessions/{sessionID}`;
- `POST /api/v1/auth/sessions/revoke-all`.

Browser route не должен принимать subject/actor из query/form. Target session ID допустим только как ID внутри уже аутентифицированного self-only actor scope. First-login должен перенаправляться на `/password/change`. Current-session revoke и revoke-all должны очищать session cookie и возвращать пользователя к login.

Этот source slice не добавляет SQL migration, dependency, новую permission, generic execution, production mutation, external publication или commercial entitlement.
