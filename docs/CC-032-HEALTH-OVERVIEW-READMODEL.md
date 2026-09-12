# Control Center 0.32 — Health Overview read model

Статус: exact-current-main qualification candidate / non-production.

## Цель

Этот slice реализует независимую часть milestone 0.32 `Health / Incidents / Audit / Reports`: детерминированное read-only представление Health, которое не показывает устаревшее, просроченное или недоступное evidence как `Healthy`.

Контракт: `ui.health-overview/v1`.

## Реализовано

- bounded provider-neutral `HealthSignal` без raw provider payload/error, credentials, worker identity и transport details;
- trusted-clock freshness boundary через `stale_after` и `expired_after`;
- `current + healthy -> healthy`;
- `stale + healthy -> degraded`;
- `expired + healthy -> unknown`;
- observed `degraded/unhealthy/unknown` не улучшается из-за старения evidence;
- unavailable source и успешно загруженный пустой набор не становятся `healthy`;
- duplicate IDs и future timestamps отклоняются fail-closed;
- signal/resource/check/runbook/evidence-reference fields имеют bounded UTF-8 boundary и отклоняют управляющие символы;
- evidence refs bounded, trimmed, deduplicated и сортируются;
- deterministic worst-state/worst-freshness aggregation по resource;
- deterministic risk-first sorting;
- `mutation_authorized=false` фиксирован в модели и JSON Schema;
- closed JSON Schema синхронизирована с bounded/control-character boundary runtime-модели;
- focused negative/freshness/aggregation/security tests входят в exact-current-main qualification candidate.

## Ограничения

Slice не:

- создаёт, acknowledge или закрывает Incident;
- запускает remediation;
- меняет Desired State;
- выдаёт Change/Job execution authority;
- добавляет generic shell/command surface;
- добавляет SQL migration или новый runtime dependency;
- активирует отдельный HTTP/UI route;
- меняет Stable/RC identity.

Любые будущие acknowledgement/remediation действия должны проходить отдельную цепочку RBAC → exact Change/approval → Job → verification → Audit/recovery.

## Интеграционная граница

В текущем canonical `main` уже присутствуют квалифицированные 0.32 Incidents, bounded Audit CSV export, Reports / Evidence Drawer contracts и 0.31 → 0.32 upgrade-preservation evidence. Этот candidate добавляет именно Health projection поверх актуального `main`, не возвращая устаревшую историю подготовительных веток.

Cross-contract адаптеры Health/Incidents/Audit → Reports, authenticated resource binding и окончательная UI/accessibility activation остаются отдельными follow-up slices и не заявляются готовыми этим изменением.

## Qualification

Для exact head требуется один штатный Public CI pass без `workflow_dispatch`, synthetic load, duplicate check или бессмысленного rerun. Обязательные существующие gates должны оставаться зелёными: Public safety, format/vet, unit/contracts, build, PostgreSQL 15–18 clean-install/supported-upgrade/adapters и race/restart.

PASS этого slice является source/read-model qualification и не означает Release Candidate или Public Stable 0.32.0. Public Stable остаётся 0.31.0 до полного завершения release train по authoritative Roadmap.
