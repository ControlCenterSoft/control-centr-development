# Control Center 0.32 — handoff в runner release-поток

Статус: **SOURCE PREPARED / НЕ ЗАПУСКАТЬ КАК ОТДЕЛЬНЫЙ RERUN**.

Этот work package подготовлен без GitHub-hosted runner и намеренно не открывает PR, не меняет `main`, не повышает `VERSION` и не выдаёт release/publication authority.

## Что подготовлено

- `internal/releasecandidate/release032.go` — exact-bound readiness/artifact policy для 0.32 поверх текущего Public Stable 0.31.1.
- `internal/releasecandidate/release032_test.go` — fail-closed тестовый код policy, включая отказ от superseded 0.31.0 Stable identity.
- `api/release-candidate-readiness-0.32-v1.schema.json` — closed public schema readiness snapshot, жёстко связанная с `v0.31.1` и его Linux artifact SHA-256.
- `third_party/manifest-0.32.json` + `scripts/generate-sbom-032.py` — dependency/license/SBOM evidence, заново привязанное к 0.32.
- `scripts/build-candidate-032.sh` — reproducible exact-SHA candidate packaging.
- `scripts/qualify-candidate-032.sh` — clean install, exact Stable 0.31.1 → 0.32 upgrade, immutable migration check, snapshot rollback и forward recovery.
- `scripts/qualify-scope-032.py` — source-bound проверка полного Health / Incidents / Audit / Reports scope; до интеграции обязательных browser/provider линий она обязана fail closed.
- `scripts/aggregate-release-evidence-032.py` — aggregation уже существующих CI results + exact scope/artifact evidence без дублирования upstream checks; Stable evidence дополнительно сверяется по версии и artifact digest.
- `.github/workflows/cc032-release-gate.yml` — reusable `workflow_call`, который не имеет собственного `push`, `pull_request` или `workflow_dispatch` trigger.
- `.github/workflows/publish-release.yml` — подготовлена отдельная ветвь проверки 0.32 evidence; future versions по-прежнему fail closed.
- release/readiness/upgrade документация 0.32.

## Canonical Stable 0.31.1 boundary

Текущий официальный Public Stable — `v0.31.1`, опубликованный как corrective patch без расширения feature/schema scope 0.31. Его canonical Linux artifact:

`control-center-0.31.1-linux-amd64.tar.gz`

имеет immutable SHA-256:

`b9d6467c7c95a6e7e8597398c1b6e7327d319058d248e9cd0416c5baf9699c97`.

Tag/source identity: `v0.31.1` / `3c9999fb5b056dad9d8dda9cf45809acb021497e`.

В отличие от исторического 0.31.0 archive, 0.31.1 package уже содержит штатный executable `scripts/migrate.sh`. Поэтому 0.32 qualifier не восстанавливает helper из raw source и не использует прежний 0.31.0 workaround. Он обязан скачать exact 0.31.1 archive, проверить pinned SHA-256, package `VERSION`, наличие executable migration runner и byte-for-byte immutable migrations `0001`–`0012` перед upgrade.

`v0.31.0` остаётся immutable исторической identity, но после выпуска 0.31.1 не является допустимой canonical Stable base для нового 0.32 release evidence.

## Когда runner-поток может использовать work package

Только после того, как exact release candidate содержит весь обязательный 0.32 scope. Открытые PR/ветки не считаются частью release scope до собственной qualification и integration. На момент последней сверки open PR #207 с Incident browser UI остаётся отдельным runner-потоком и не должен дублироваться этим package.

Runner-поток должен сначала сверить текущий `main`; PASS/branch SHA из прошлого прохода нельзя переносить на новый candidate.

## Как подключить без duplicate CI

Не добавлять отдельный synthetic workflow и не повторять Public CI matrix.

В том же release-identity change, где финальный `main` получает `VERSION=0.32.0` и соответствующий runtime version, существующий `Public CI` должен после обычных jobs (`public-safety`, `static-analysis`, `unit`, `build`, PostgreSQL migration/adapters, `race`) вызвать reusable workflow `.github/workflows/cc032-release-gate.yml` и передать:

- exact `${{ github.sha }}` / PR head SHA согласно canonical CI identity policy;
- фактические `needs.<job>.result` существующих jobs.

Reusable gate повторно **не запускает** эти generic checks. Он выполняет только release-specific scope qualification, packaging, official Stable 0.31.1 artifact upgrade/rollback и aggregation already-produced upstream results.

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

- не переносить PASS 0.31.0, 0.31.1 или другого 0.32 SHA на новый candidate;
- не использовать superseded `v0.31.0` как текущую Stable base;
- не редактировать опубликованные Stable migrations;
- не считать open PR/ветку частью scope до integration;
- не обходить Health/Incident/Audit/Reports source qualifier;
- не запускать отдельный duplicate Public CI ради release gate;
- не публиковать 0.32 без exact artifact/readiness evidence из того же candidate SHA;
- не менять tag/release identity задним числом.
