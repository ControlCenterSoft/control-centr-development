# Control Center 0.32 — qualification обновления с Public Stable 0.31

Статус: release-gate evidence для линии 0.32. Этот документ не объявляет 0.32 Release Candidate или Public Stable.

## Цель

Зафиксировать точную границу обновления от опубликованного Control Center 0.31.0 к текущей 0.32 schema line. Public Stable 0.31 включает migrations `0001`–`0012`; первая новая migration линии 0.32 — `0013_incident_read_models`.

Authoritative Stable baseline для artifact-level qualification:

- release/tag: `v0.31.0`;
- Linux artifact: `control-center-0.31.0-linux-amd64.tar.gz`;
- SHA-256: `0b270edcf1d17bd6a38fa3f77b78c4112d43fb945582ee2d25cd91daf38cf06c`.

Другой digest или изменённая Stable identity не может использоваться как evidence перехода 0.31 → 0.32.

Qualification обязана доказать, что добавление incident persistence не переписывает ранее опубликованное состояние и не ослабляет существующие Identity/RBAC/Job/Audit/recovery границы.

## Проверяемый state-preservation сценарий

`TestPostgresUpgradeFrom031PreservesIdentityAndJobState` выполняется на одноразовой PostgreSQL-базе и проверяет:

1. установку точного Stable 0.31 migration boundary `0001`–`0012`;
2. создание локального `admin`, смену bootstrap-пароля и сохранение `password_change_required=false`;
3. существующие immutable revision, Change, durable Job и Job timeline до обновления;
4. применение `0013_incident_read_models.up.sql`;
5. появление только 0.32 incident tables без изменения существующего 0.31 состояния;
6. идемпотентный replay `0013`;
7. scoped rollback только `0013`, при котором 0.31 состояние остаётся доступным и неизменным;
8. повторное применение `0013` и PostgreSQL restart с повторной проверкой состояния.

## Artifact lifecycle qualification

Подготовленный `scripts/qualify-candidate-032.sh` дополняет state-preservation test и должен выполняться только после формирования exact 0.32 release identity. Он обязан:

- собрать exact candidate artifact с `VERSION=0.32.0` и точным `REVISION`;
- проверить SHA-256 sidecar, source archive, CycloneDX SBOM и THIRD_PARTY_NOTICES;
- сравнить каждый опубликованный Stable `*.up.sql` byte-for-byte с одноимённым candidate migration;
- выполнить clean install 0.32;
- установить schema из официального 0.31 artifact и обновить её exact 0.32 candidate migrations;
- сохранить exact pre-upgrade database snapshot;
- восстановить snapshot и доказать отсутствие candidate-only migration;
- повторно применить тот же exact candidate как forward recovery;
- сформировать candidate qualification/provenance/release-manifest без publication authority;
- проверить immutable expected artifact set через `ValidateArtifactManifest032`.

Подготовка этого скрипта не является PASS: runner evidence появляется только после его фактического выполнения на точном candidate SHA.

## Release boundary

PASS state-preservation и artifact lifecycle закрывает только migration/install/upgrade/rollback часть 0.32. Для Public Stable 0.32 отдельно остаются обязательными полный Roadmap scope, exact-head qualification, PostgreSQL adapter/restart/reconnect, security/privacy, release metadata/provenance/checksums и version-specific Stable promotion gate.

Commercial/legal документы ведутся отдельным commercial-launch track и не преобразуются в фиктивный технический PASS.
