# Control Center 0.32 — Reports / Evidence Drawer

Статус: **runner-free source preparation / non-production**.

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
- JSON Schema для обоих public read contracts подготовлены;
- focused tests подготовлены и локально, вне GitHub runner, прошли `gofmt` + `go test`.

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

Этот branch не дублирует существующие ветки по:

- Health Overview;
- Incident persistence/correlation/operator service/read model;
- bounded Audit export/redaction.

При интеграции runner-поток должен адаптировать эти источники к `ReportEvidenceInput`, сохраняя их собственную authoritative freshness/integrity семантику. Report builder не имеет права считать отсутствие upstream evidence подтверждением здоровья.

## Release boundary

Slice не меняет текущую Stable/RC identity, `VERSION`, SQL migrations, runtime dependencies, deployment authority, permissions или коммерческое/legal disposition 0.31.

До интеграции в 0.32 необходимо отдельным runner-потоком:

1. rebased/merge qualified 0.32 integration head;
2. `gofmt`, `go vet`, полный `go test ./...` и race qualification;
3. JSON Schema validation;
4. cross-contract tests Health + Incidents + Audit + Report/Evidence Drawer;
5. HTTP adapter с server-side authenticated scope/resource binding;
6. negative tests на unavailable/stale/expired upstream evidence и отсутствие raw sensitive data;
7. UI/accessibility tests Evidence Drawer и Reports;
8. один штатный exact-head CI pass без duplicate rerun.

Ветка намеренно не имеет PR: `pull_request` запускает GitHub Actions, что запрещено для данного non-runner прохода. Push остаётся только в `work/**`, который не входит в `Public CI` push filters.
