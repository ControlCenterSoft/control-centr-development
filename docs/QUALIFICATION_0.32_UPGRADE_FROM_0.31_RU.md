# Control Center 0.32 — qualification обновления с Public Stable 0.31.1

Статус: release-gate evidence для линии 0.32. Этот документ не объявляет 0.32 Release Candidate или Public Stable.

## Цель

Зафиксировать точную границу обновления от текущего опубликованного Control Center 0.31.1 к 0.32 schema line. Public Stable 0.31.1 сохраняет тот же feature/schema scope 0.31 и migrations `0001`–`0012`; первая новая migration линии 0.32 — `0013_incident_read_models`.

Authoritative Stable baseline для artifact-level qualification:

- release/tag: `v0.31.1`;
- Linux artifact: `control-center-0.31.1-linux-amd64.tar.gz`;
- SHA-256: `b9d6467c7c95a6e7e8597398c1b6e7327d319058d248e9cd0416c5baf9699c97`;
- source release commit: `3c9999fb5b056dad9d8dda9cf45809acb021497e`;
- package contract содержит штатный executable `scripts/migrate.sh` и не требует исторической reconstruction-процедуры 0.31.0.

Другой digest, предыдущий `v0.31.0` либо изменённая Stable identity не могут использоваться как evidence текущего перехода 0.31.1 → 0.32. Исторический `v0.31.0` остаётся immutable, но после выпуска corrective Stable 0.31.1 больше не является canonical upgrade base.

Qualification обязана доказать, что добавление incident persistence не переписывает ранее опубликованное состояние и не ослабляет существующие Identity/RBAC/Job/Audit/recovery границы.

## Проверяемый state-preservation сценарий

`TestPostgresUpgradeFrom031PreservesIdentityAndJobState` проверяет неизменяемый schema family 0.31 и остаётся применимым к 0.31.1, поскольку corrective patch не добавляет migration. На одноразовой PostgreSQL-базе проверяются:

1. установка точного Stable 0.31.1 migration boundary `0001`–`0012`;
2. создание локального `admin`, смена bootstrap-пароля и сохранение `password_change_required=false`;
3. существующие immutable revision, Change, durable Job и Job timeline до обновления;
4. применение `0013_incident_read_models.up.sql`;
5. появление только 0.32 incident tables без изменения существующего 0.31.1 состояния;
6. идемпотентный replay `0013`;
7. scoped rollback только `0013`, при котором 0.31.1 состояние остаётся доступным и неизменным;
8. повторное применение `0013` и PostgreSQL restart с повторной проверкой состояния.

## Artifact lifecycle qualification

Подготовленный `scripts/qualify-candidate-032.sh` дополняет state-preservation test и должен выполняться только после формирования exact 0.32 release identity. Он обязан:

- собрать exact candidate artifact с `VERSION=0.32.0` и точным `REVISION`;
- проверить SHA-256 sidecar, source archive, CycloneDX SBOM и THIRD_PARTY_NOTICES;
- скачать именно immutable `control-center-0.31.1-linux-amd64.tar.gz`, проверить pinned SHA-256 и package `VERSION`;
- требовать штатный executable `scripts/migrate.sh` непосредственно внутри 0.31.1 artifact, без восстановления helper из другого источника;
- сравнить каждый опубликованный Stable `*.up.sql` byte-for-byte с одноимённым candidate migration;
- выполнить clean install 0.32;
- установить schema из официального 0.31.1 artifact и обновить её exact 0.32 candidate migrations;
- сохранить exact pre-upgrade database snapshot;
- восстановить snapshot и доказать отсутствие candidate-only migration;
- повторно применить тот же exact candidate как forward recovery;
- сформировать candidate qualification/provenance/release-manifest, явно привязанные к Stable 0.31.1, без publication authority;
- проверить immutable expected artifact set через `ValidateArtifactManifest032`.

Подготовка этого скрипта не является PASS: runner evidence появляется только после его фактического выполнения на точном candidate SHA.

## Release boundary

PASS state-preservation и artifact lifecycle закрывает только migration/install/upgrade/rollback часть 0.32. Для Public Stable 0.32 отдельно остаются обязательными полный Roadmap scope, exact-head qualification, PostgreSQL adapter/restart/reconnect, security/privacy, release metadata/provenance/checksums и version-specific Stable promotion gate.

Commercial/legal документы ведутся отдельным commercial-launch track и не преобразуются в фиктивный технический PASS.
