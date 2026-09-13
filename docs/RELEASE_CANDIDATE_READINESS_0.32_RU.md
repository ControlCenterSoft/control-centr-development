# Control Center 0.32.0 — Release Candidate readiness

Статус: **PREPARED / НЕ RELEASE CANDIDATE / НЕ PUBLIC STABLE**.

Текущий канонический Public Stable — **0.31.0** (`v0.31.0`). Его Linux artifact имеет SHA-256 `0b270edcf1d17bd6a38fa3f77b78c4112d43fb945582ee2d25cd91daf38cf06c`. Любое release evidence 0.32 обязано быть привязано к одной точной immutable candidate revision и к этой опубликованной Stable-базе; evidence другого SHA или другой Stable identity не переносится.

## Обязательный продуктовый scope 0.32

0.32 закрывает milestone **Health / Incidents / Audit / Reports** из canonical Roadmap. До Release Candidate должны быть интегрированы и квалифицированы на точном итоговом SHA:

- Health Overview с fail-closed freshness: stale/expired/unavailable не становятся Healthy;
- Incidents read model, persistence и operator boundary;
- Audit bounded search/export и browser boundary без SourceIP/raw Details в operator view;
- Reports / Evidence Drawer с resource-bound evidence и без cross-resource leakage;
- authoritative adapters Health / Incidents / Audit → Reports;
- authenticated server-side RBAC и first-login/password-change boundary для всех активированных API/Web маршрутов;
- поддерживаемый upgrade 0.31 → 0.32 с immutable migrations 0.31 и additive 0.32 schema;
- отсутствие false Success, hidden mutation authority и client-controlled actor/provider/source selection.

Подготовленные, но ещё не интегрированные ветки/PR не являются release evidence. В частности, открытый Incident browser PR и отдельная Health HTTP/browser/provider линия должны либо войти в точный кандидат после собственной qualification, либо быть явно исключены из заявленного 0.32 scope через authoritative Roadmap change. Их наличие само по себе не закрывает gate.

## Технические gates Product Public Stable

Точный 0.32 candidate должен получить PASS по каждому техническому gate:

1. `health_incidents_audit_reports_integration` — canonical scope 0.32 интегрирован, runtime/API/Web boundaries соответствуют authoritative Roadmap и не содержат неподтверждённых success claims.
2. `candidate_artifact_packaging` — exact-SHA reproducible Linux/source artifacts, SBOM, THIRD_PARTY_NOTICES, checksums, provenance и release manifest.
3. `clean_install` — чистая установка exact candidate artifact.
4. `upgrade_from_stable_0_31` — поддерживаемое обновление с опубликованного 0.31.0 с сохранением данных, настроек, установленного admin password и first-login state.
5. `rollback_forward_recovery` — восстановление exact pre-upgrade state и повторное безопасное forward recovery.
6. `postgres_restart_reconnect` — PostgreSQL 15–18 / adapter / restart-reconnect boundaries для нового durable state.
7. `security_privacy` — RBAC, no-secret/privacy, stale/idempotency, Audit integrity, browser security headers, CSV/formula safety и negative/failure paths.
8. `release_metadata` — VERSION/runtime identity/release notes/artifact manifests/checksums/provenance согласованы с одной exact candidate revision.

`commercial_legal_clearance` сохраняется отдельным gate полного коммерческого запуска. Отсутствие юридического пакета не превращается в PASS и не позволяет заявлять коммерческие/договорные гарантии, но по принятой политике не является техническим Product Public Stable blocker при полностью закрытых product gates.

## Packaging boundary

Подготовлен отдельный 0.32 artifact contract. Ожидаемый immutable set:

- `control-center-0.32.0-linux-amd64.tar.gz`;
- `control-center-0.32.0-linux-amd64.tar.gz.sha256`;
- `control-center-0.32.0-source.tar.gz`;
- `control-center-0.32.0.sbom.cdx.json`;
- `THIRD_PARTY_NOTICES.md`;
- `control-center-0.32.0.provenance.json`;
- `control-center-0.32.0.qualification.json`;
- `control-center-0.32.0.release-manifest.json`;
- `SHA256SUMS`.

0.31 artifact names, digests или readiness snapshot не могут использоваться как 0.32 evidence. Уже опубликованные 0.31 migrations должны совпадать byte-for-byte; 0.32 schema changes добавляются только новой migration.

## Stop conditions

Promotion запрещён, если остаётся хотя бы одно из следующего:

- candidate VERSION/revision/artifact identity расходится;
- обязательный technical gate отсутствует, pending или blocked;
- source/migration drift затрагивает опубликованную 0.31 базу;
- stale/malformed/cross-resource evidence может быть показано как current/Healthy/Success;
- upgrade сбрасывает пользовательские данные, настройки или установленный пароль;
- rollback/forward recovery не доказан;
- high-risk security/privacy/recovery defect;
- обязательный artifact/checksum/provenance set неполон или не связан с exact SHA.

Этот документ не запускает qualification и не выдаёт publication authority. Он определяет fail-closed критерий для последующего runner-потока.
