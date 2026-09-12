# Control Center 0.31 — сохранение browser authentication boundary из Stable

Статус: **release-blocking regression prevention / требует exact-head qualification**.

Перед продвижением 0.31 в PUBLIC STABLE обнаружено, что текущий Stable-канал 0.30 содержит отдельное квалифицированное исправление browser/API authentication boundary, которое отсутствовало в опубликованном исходном snapshot `v0.31.0`.

Без переноса этого исправления promotion привёл бы к функциональной регрессии браузерного интерфейса: HTML-маршруты могли возвращать JSON authentication envelope API вместо browser redirect, а mandatory first-login password-change flow не сохранял бы отдельную web-семантику.

## Переносимая граница

В 0.31 переносится уже используемое Stable-поведение:

- `GET /password/change` и `POST /web/password/change` используют browser-only `AuthenticateWeb`;
- `GET /overview` использует `AuthenticateWeb` и `RequireWeb`;
- неаутентифицированный browser request перенаправляется на `/login` и не получает API JSON error contract;
- invalid/expired browser session очищается и переводится на `/login`;
- пользователь с обязательной сменой первоначального пароля направляется на `/password/change`;
- API authentication/authorization contract остаётся прежним и продолжает выдавать bounded JSON `401/403`;
- denied browser authorization не раскрывает API error envelope.

Продуктовый delta `internal/identity/httpapi/server.go` идентичен ранее квалифицированному Stable hotfix; дополнительный focused regression test подтверждает перенос этого behavior в текущую 0.31 source line.

## Release boundary

Официальный development tag `v0.31.0` остаётся immutable и не перемещается. После qualification этого исправления дальнейший PUBLIC STABLE promotion обязан использовать явно проверяемую promotion lineage от опубликованного `v0.31.0` к exact qualified descendant, включающему этот regression fix. Нельзя молча продвигать более старый source snapshot без исправления.

Изменение не добавляет SQL migration, dependency, permission, generic execution authority или новый credential path. Любой failure focused/full CI считается release blocker.
