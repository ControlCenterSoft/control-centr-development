# Control Center 0.32 — Reports / Evidence Drawer HTTP boundary

Статус: **EXACT-CURRENT-MAIN QUALIFICATION CANDIDATE / SOURCE-ONLY / NOT PUBLIC STABLE**.

Этот source-only slice готовит следующий непокрытый слой milestone 0.32 после квалифицированных Incidents, Audit export, Health Overview, Reports/Evidence Drawer contracts и Health/Incidents/Audit → Reports adapters. Он намеренно **не регистрирует production route** и поэтому не создаёт неподтверждённую activation claim.

## Модель

`OperationalReportProvider` представляет ровно один уже авторизованный read-only источник `ui.operational-report/v1`. HTTP adapter не выбирает источник по пользовательскому параметру: конкретный provider привязывается сервером при регистрации route. Это исключает переход от разрешённого Health/Resources отчёта к Audit evidence через client-controlled `source=audit`.

Перед выдачей результата `ValidateOperationalReport` повторно строит canonical projection из допустимого upstream evidence и требует точного совпадения schema, data state, ordering, effective health state, evidence digest и `mutation_authorized=false`. Tampered, malformed или non-canonical provider output становится `503`, а не частичным/Healthy результатом.

Evidence Drawer принимает только точную пару `resource_kind` + `resource_id`. Unknown, duplicate или неполные query parameters отклоняются. Drawer строится штатным `BuildEvidenceDrawer`, поэтому evidence другого ресурса не попадает в ответ.

## RBAC / security boundary

Outer route registration остаётся отдельным integration gate и обязана сохранить permission источника:

- Health / infrastructure evidence может быть опубликовано только в границе, где authoritative provider разрешён текущему субъекту (минимум существующая `resources.read`, если source contract действительно ограничен этим scope);
- Audit-derived Reports/Evidence Drawer должны оставаться за существующим global `audit.events.read` и mandatory first-login password-change boundary;
- нельзя объединять Audit evidence и ordinary resource evidence под более слабым разрешением только из-за общего `ui.operational-report/v1` формата;
- client не выбирает actor, permission, provider или source;
- HTTP слой GET-only, `no-store`, `nosniff`, без mutation/execution/remediation authority;
- provider error, unavailable state или failed canonical validation работают fail-closed.

## Qualification

Ветка основана непосредственно на текущем canonical `main` после интеграции PR #201 и содержит только Reports/Evidence Drawer provider/validator, HTTP adapter, focused negative/security tests и этот документ. Для exact head требуется один штатный `pull_request` Public CI: public-safety, format/vet, unit/contract, build, PostgreSQL 15–18 clean-install/supported-upgrade/adapters и race/restart. Не запускать `workflow_dispatch`, synthetic load, duplicate checks или unchanged rerun; не отменять и не вытеснять другие Control Center jobs.

PASS этого прохода квалифицирует только source HTTP boundary. Production route registration, точный authoritative provider и его RBAC permission, route reachability, anonymous/first-login/RBAC denial, UI/accessibility и cross-source privilege-widening checks остаются отдельным последующим activation gate.

## Что подготовлено для следующего runner-потока

Добавлены provider contract, canonical validator, GET-only Operational Report handler, resource-bound Evidence Drawer handler и negative/security test code. Перед production activation следующий поток должен выбрать точные route names, связать каждый route с authoritative provider и соответствующим RBAC permission, затем квалифицировать route reachability, anonymous/first-login/RBAC denial, resource binding, accessibility/UI integration и отсутствие cross-source privilege widening.

Этот slice не меняет `VERSION`, SQL migrations, dependencies, release metadata или Public Stable 0.31.0 и не заявляет 0.32 RC/Stable readiness. Commercial/legal launch остаётся отдельным контуром; техническая граница не превращает отсутствующее commercial evidence в PASS.
