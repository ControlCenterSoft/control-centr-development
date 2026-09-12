# Control Center 0.32 — Reports / Evidence Drawer

Статус: **EXACT-CURRENT-MAIN QUALIFICATION / SOURCE-ONLY / NOT PUBLIC STABLE**.

## Цель

Этот slice закрывает ранее непокрытую часть milestone 0.32 `Health / Incidents / Audit / Reports`: безопасную read-only агрегацию operational evidence и resource-bound Evidence Drawer. Он не заменяет Health/Incident/Audit источники и не повышает их доверие — только проецирует уже полученное evidence с fail-closed freshness semantics.

Контракты:

- `ui.operational-report/v1`;
- `ui.evidence-drawer/v1`.

## Реализовано

- bounded operational report: максимум 1000 evidence items;
- resource-bound Evidence Drawer без cross-resource leakage;
- `healthy + stale -> degraded`;
- `healthy + expired/unavailable -> unknown`;
- `degraded`, `unhealthy` и `unknown` никогда не улучшаются из-за projection;
- unavailable source и loaded-empty source не становятся `healthy`;
- deterministic risk-first ordering;
- deterministic SHA-256 digest нормализованного report evidence;
- duplicate evidence IDs и duplicate evidence refs отклоняются;
- future `observed_at`, invalid state/freshness и malformed digest отклоняются fail-closed;
- `reason_code` принимает только bounded machine-safe token вместо raw provider/runtime error text;
- `runbook_ref`, resource/evidence refs bounded и не допускают control characters;
- `mutation_authorized=false` является жёсткой границей обоих контрактов;
- JSON Schema для обоих public read contracts включены;
- focused negative/freshness/resource-binding tests включены.

## Security / privacy boundary

Slice сознательно не принимает и не возвращает:

- credentials, tokens, passwords или password hashes;
- raw provider payload;
- raw command/job output;
- raw exception/error body;
- customer/free-form note как evidence;
- deployment/mutation authority.

Human-readable локализованный текст должен строиться UI по безопасному `reason_code`, а не переносить в report произвольный backend/provider text.

## Связь с параллельными 0.32 slices

Этот qualification branch не дублирует текущую отдельную линию Health Overview и уже интегрированные Incident/Audit slices. Источники Health/Incidents/Audit должны адаптироваться к `ReportEvidenceInput`, сохраняя собственную authoritative freshness/integrity семантику. Report builder не имеет права считать отсутствие upstream evidence подтверждением здоровья.

## Qualification boundary

Текущий branch создан от exact current `main` после интеграции bounded Audit CSV route. Один штатный Public CI должен подтвердить:

1. formatting/vet/build/unit-contract;
2. PostgreSQL 15–18 clean-install/supported-upgrade/adapters и restart/race regressions всего текущего продукта;
3. focused fail-closed report tests;
4. отсутствие новых credentials/secrets/public-safety нарушений.

Этот проход квалифицирует **source/read-model contracts**, но не заявляет HTTP/UI activation или Public Stable. Для пользовательской активации отдельно остаются authenticated server-side scope/resource binding, cross-contract Health/Incidents/Audit adapters и UI/accessibility qualification. Эти последующие slices не должны дублировать уже пройденную source qualification без содержательного изменения exact head.

## Release boundary

Slice не меняет текущую Public Stable 0.31.0 identity, `VERSION`, SQL migrations, runtime dependencies, deployment authority, permissions или commercial/legal disposition. Он является bounded 0.32 development evidence и не может самостоятельно разрешать release/promotion.
