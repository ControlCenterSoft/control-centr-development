# Control Center 0.32 — handoff в runner release-поток

Статус: **SOURCE PREPARED / НЕ ЗАПУСКАТЬ КАК ОТДЕЛЬНЫЙ RERUN**.

Этот work package подготовлен без GitHub-hosted runner и намеренно не открывает PR, не меняет `main`, не повышает `VERSION` и не выдаёт release/publication authority.

## Что подготовлено

- `internal/releasecandidate/release032.go` — exact-bound readiness/artifact policy для 0.32 поверх Public Stable 0.31.0.
- `internal/releasecandidate/release032_test.go` — fail-closed тестовый код policy.
- `api/release-candidate-readiness-0.32-v1.schema.json` — closed public schema readiness snapshot.
- `third_party/manifest-0.32.json` + `scripts/generate-sbom-032.py` — dependency/license/SBOM evidence, заново привязанное к 0.32.
- `scripts/build-candidate-032.sh` — reproducible exact-SHA candidate packaging.
- `scripts/qualify-candidate-032.sh` — clean install, exact Stable 0.31 → 0.32 upgrade, immutable migration check, snapshot rollback и forward recovery.
- `scripts/qualify-scope-032.py` — source-bound проверка полного Health / Incidents / Audit / Reports scope; до интеграции Health и Incident browser line обязана fail closed.
- `scripts/aggregate-release-evidence-032.py` — aggregation уже существующих CI results + exact scope/artifact evidence без дублирования upstream checks.
- `.github/workflows/cc032-release-gate.yml` — reusable `workflow_call`, который не имеет собственного `push`, `pull_request` или `workflow_dispatch` trigger.
- `.github/workflows/publish-release.yml` — подготовлена отдельная ветвь проверки 0.32 evidence; future versions по-прежнему fail closed.
- release/readiness/upgrade документация 0.32.

## Известная Stable 0.31 package boundary

Опубликованный `control-center-0.31.0-linux-amd64.tar.gz` имеет immutable SHA-256:

`0b270edcf1d17bd6a38fa3f77b78c4112d43fb945582ee2d25cd91daf38cf06c`

В этом историческом binary archive отсутствует `scripts/migrate.sh`. 0.32 qualifier **не исправляет опубликованный архив**: он восстанавливает migration runner только из exact tag `v0.31.0` и требует pinned SHA-256 `6965ea551bddd809bc23eee17ed56b828a71b0d294761f0210bb9a6dc8c12f3d`, как canonical Stable verification path. Любой другой archive shape/helper fail closed.

## Когда runner-поток может использовать work package

Только после того, как exact release candidate содержит весь обязательный 0.32 scope. На момент подготовки source-only work отдельные Health HTTP/Web/provider изменения и Incident browser изменения ещё не должны считаться release evidence до собственной qualification/integration.

Runner-поток должен сначала сверить текущий `main`; PASS/branch SHA из прошлого прохода нельзя переносить на новый candidate.

## Как подключить без duplicate CI

Не добавлять отдельный synthetic workflow и не повторять Public CI matrix.

В том же release-identity change, где финальный `main` получает `VERSION=0.32.0` и соответствующий runtime version, существующий `Public CI` должен после обычных jobs (`public-safety`, `static-analysis`, `unit`, `build`, PostgreSQL migration/adapters, `race`) вызвать reusable workflow `.github/workflows/cc032-release-gate.yml` и передать:

- exact `${{ github.sha }}` / PR head SHA согласно canonical CI identity policy;
- фактические `needs.<job>.result` существующих jobs.

Reusable gate повторно **не запускает** эти generic checks. Он выполняет только release-specific scope qualification, packaging, official Stable 0.31 artifact upgrade/rollback и aggregation already-produced upstream results.

Evidence artifact должен называться:

`control-center-0.32.0-release-evidence-<exact SHA>`.

Именно его `publish-release.yml` ищет в том же successful Public CI run.

## Release notes и exact identity

До финального main qualification `docs/RELEASE_0.32.0_RU.md` остаётся `PREPARED`.

В release-identity change перед exact qualification строка статуса должна стать **ровно**:

`Статус: **PUBLIC STABLE RELEASE**.`

и из файла должны быть удалены candidate-only формулировки `PREPARED`, `НЕ RELEASE CANDIDATE`, `НЕ PUBLIC STABLE`, `candidate-only`.

`publish-release.yml` для 0.32 проверяет это fail closed. Изменять release notes после получения exact-SHA evidence нельзя: это создаёт новый candidate SHA и требует новой qualification.

## Commercial/legal boundary

Readiness snapshot сохраняет `commercial_legal_clearance=blocked`, если внешний legal/commercial review ещё не завершён. `EvaluateProductStable032` требует все технические gates, но не подменяет legal evidence фиктивным PASS. `Evaluate032` при таком состоянии обязан иметь единственный blocker `commercial_legal_clearance`.

Это соответствует принятому разделению Product Public Stable и commercial launch.

## Запрещённые shortcut

- не переносить PASS 0.31 или другого 0.32 SHA;
- не редактировать опубликованные Stable migrations;
- не считать open PR/ветку частью scope до integration;
- не обходить Health/Incident/Audit/Reports source qualifier;
- не запускать отдельный duplicate Public CI ради release gate;
- не публиковать 0.32 без exact artifact/readiness evidence из того же candidate SHA;
- не менять tag/release identity задним числом.
