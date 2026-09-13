# Control Center 0.32 — Audit search / export Web UI

Статус: **NON-RUNNER SOURCE PREPARATION / НЕ КВАЛИФИЦИРОВАНО / НЕ PUBLIC STABLE**.

Этот slice продолжает фактическую линию 0.32 после интеграции Incidents, bounded Audit CSV export, Health Overview, Reports / Evidence Drawer, source-specific Reports API/RBAC и browser Reports routes. Он закрывает оставшуюся пользовательскую часть milestone «Audit search/export UI» без изменения Audit storage, permissions или execution authority.

## Реализуемая граница

- браузерный маршрут `GET /web/audit`;
- только существующее глобальное разрешение `audit.events.read`;
- browser-auth semantics через `AuthenticateWeb` / `RequireWeb`, включая обязательную смену bootstrap-пароля;
- точные серверные фильтры `action`, `outcome`, `actor_id`, `subject_id`, bounded `limit` и opaque pagination cursor;
- чтение через существующий authoritative `audit.Reader` и тот же `parseAuditQuery`, что используется API;
- privacy-minimized представление: browser UI не показывает `SourceIP` и `Details`;
- перед возвратом HTML обязательно сохраняется отдельное Audit evidence `audit.events_web`; при невозможности записать evidence данные не выдаются;
- ссылка на уже квалифицированный bounded CSV export сохраняет текущие точные фильтры/страницу и не создаёт новый bulk/background export path;
- состояния и действия обозначаются текстом, UI не опирается только на цвет;
- read-only surface: mutation/execution/remediation/publication authority отсутствует.

## Security boundary

Клиент не выбирает actor для авторизации, permission или источник Audit. `actor_id` является только точным фильтром уже разрешённого append-only журнала. Viewer без `audit.events.read` получает отказ. Пользователь с обязательной сменой первоначального пароля перенаправляется на `/password/change`. Неподдерживаемые, повторённые или выходящие за bounds query-параметры отклоняются fail-closed.

В browser таблицу намеренно не проецируются `SourceIP` и `Details`, даже несмотря на то, что API имеет собственный permission-gated контракт чтения. Это уменьшает объём PII/provider-private контекста на обычной операторской поверхности. CSV export сохраняет отдельную уже существующую privacy/redaction/formula-safety границу.

## Qualification boundary

Ветка `work/cc-0.32-audit-search-ui-nr2-20260913` является non-runner подготовкой. В рамках этого прохода запрещено создавать PR, push в `main`/`release/**`/`develop/**`/`migration/**`, запускать workflow/check/rerun или иным способом инициировать GitHub Actions.

Перед интеграцией runner-поток должен выполнить exact-head qualification как минимум для:

- compile / format / vet;
- existing identity/auth/RBAC/Audit tests;
- новых Web Audit security/privacy tests;
- anonymous/viewer/bootstrap-password boundaries;
- evidence-before-render fail-closed behavior;
- pagination/filter/export-link preservation;
- post-merge main qualification без duplicate/rerun.

Public Stable на момент подготовки — **0.31.0**. Этот slice не меняет `VERSION` и не заявляет 0.32 RC/Stable readiness.
