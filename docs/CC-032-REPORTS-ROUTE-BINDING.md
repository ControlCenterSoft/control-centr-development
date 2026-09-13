# Control Center 0.32 — Reports source-specific route binding

Статус: **RUNNER QUALIFICATION CANDIDATE / NOT PUBLIC STABLE**.

Этот slice продолжает квалифицированную source HTTP boundary Reports / Evidence Drawer и закрывает следующий security gate: route registration не имеет права ослаблять authorization исходного evidence source.

## Маршруты и права

При наличии явно переданного authoritative provider регистрируются отдельные маршруты:

- `GET /api/v1/ui/reports/resources` — global `resources.read`;
- `GET /api/v1/ui/reports/resources/evidence` — global `resources.read`;
- `GET /api/v1/ui/reports/audit` — global `audit.events.read`;
- `GET /api/v1/ui/reports/audit/evidence` — global `audit.events.read`.

Client-controlled `source`, actor, permission или provider отсутствуют. Audit evidence нельзя получить через resource route. Site-scoped `viewer` не получает global report aggregate. Anonymous/unbound identities отклоняются штатной Identity/RBAC boundary.

## Fail-closed activation

Provider подключается только отдельной server-side option. Если authoritative provider отсутствует, маршрут не регистрируется и остаётся `404`; пустой/synthetic provider не создаётся. Этот slice также добавляет dispatch `/api/v1/ui/reports/...` в product router, но default runtime пока не передаёт Reports provider и поэтому не заявляет production activation.

HTTP handler сохраняет уже квалифицированные свойства source boundary: GET-only, `no-store`, `nosniff`, canonical report revalidation, resource-bound Evidence Drawer, `mutation_authorized=false` и `503` при provider/validation failure.

## Qualification

Для exact branch head нужен один штатный pull-request Public CI со всеми текущими deterministic gates: public safety, format/vet, unit/contract, build, PostgreSQL 15–18 clean-install/supported-upgrade/adapters и race/restart. Duplicate workflow, unchanged rerun, synthetic load и cancellation/displacement запрещены.

Qualification обязана доказать как минимум:

- anonymous/unbound denial;
- global scope enforcement;
- `resources.read` route доступен viewer/operator/auditor/admin в соответствии с существующими built-in roles;
- Audit route доступен auditor/admin, но не viewer/operator;
- absence of provider => route absent;
- существующий Public Stable 0.31.0, migrations, dependencies и release identity не изменены.

Следующий отдельный gate после этого slice — production authoritative provider wiring и фактическая UI/accessibility integration. Commercial/legal launch остаётся отдельным контуром и не подменяет технические security gates.
