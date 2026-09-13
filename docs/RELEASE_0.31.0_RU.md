# Control Center 0.31.0 — Changes / Jobs operational workflow

Статус: **OFFICIAL PUBLIC STABLE RELEASE**.

Control Center 0.31.0 опубликован как официальный Public Stable релиз и заменяет 0.30.0 в качестве текущей стабильной версии. Релиз вводит основной безопасный operational workflow вокруг типизированных Changes и Jobs. Коммерческий запуск и юридические документы ведутся отдельным контуром и не подменяют техническое release evidence продукта.

## Пользовательский результат

Администратор получает единый проверяемый workflow, в котором можно различать намерение, подтверждение, выполнение и подтверждённый результат:

- точная immutable revision Change;
- semantic diff и blast radius;
- execution preflight и maintenance-window evidence;
- approval state/evidence, связанное с точной revision/hash;
- durable Job lifecycle и timeline;
- reconnect с сильной version/ETag identity;
- version-bound cancel и policy-bounded manual retry;
- terminal result и post-condition evidence;
- recovery-path readiness;
- Audit/evidence, не позволяющие представить неподтверждённое состояние как Success.

Unknown, stale, incomplete, mismatched или противоречивое evidence работает fail-closed и не отображается как Healthy/Success.

## Что входит в 0.31.0

- read-only Changes / Jobs projections и authoritative runtime API boundary;
- revision-bound semantic diff и blast-radius evidence;
- execution preflight и maintenance-window evidence;
- durable Job admission перед enqueue;
- durable Job lifecycle/timeline;
- fail-closed approval и result evidence;
- exact-version cancellation/reconnect boundary;
- bounded manual-retry admission, atomic retry lineage и повторная revalidation;
- полный `approval → Job → verification → recovery` operational evidence flow;
- bounded recovery-path evidence с exact Change/revision binding и fresh verified backup/restore evidence;
- deterministic packaging, SBOM, THIRD_PARTY_NOTICES, SHA256SUMS, provenance и release manifest;
- exact-bound release evidence.

## Информационная безопасность

0.31.0 не вводит универсальный shell/command execution API. Risk-bearing mutation остаётся внутри цепочки:

`Identity/RBAC → Desired State/Change → exact approval → durable Job → typed execution → Actual State/post-condition verification → Audit/evidence → rollback/recovery`.

Обязательные свойства релиза:

- approval относится к точной revision/hash;
- Job admission повторно проверяет current revision и effective approvals непосредственно перед durable binding;
- stale Job version не может незаметно пройти cancel/retry;
- terminal `succeeded` без корректного post-condition evidence не становится подтверждённым Success;
- корректно зафиксированный failed outcome остаётся failed;
- Changes / Jobs evidence доступно только в разрешённом RBAC scope;
- secrets, credentials, raw Job input/output, lease material и private provider details не входят в operator evidence;
- published migrations предыдущих версий остаются immutable byte-for-byte;
- public repository safety и no-secret/privacy boundaries являются обязательными release gates.

## Установка, обновление и восстановление

Для опубликованного 0.31.0 квалифицированы:

- clean install;
- supported upgrade с Public Stable 0.30.0;
- сохранность пользовательских данных и настроек;
- сохранность установленного пользователем пароля `admin` и first-login/password-change state;
- PostgreSQL 15/16/17/18 migration и adapter paths;
- restart/reconnect и race qualification;
- rollback через сохранённый pre-upgrade state и последующий forward recovery;
- reproducible release artifacts и checksums.

Обновление не сбрасывает установленный пароль администратора к `admin`. Первоначальный `admin/admin` используется только на чистой установке и требует обязательной смены при первом входе.

Multi-node/HA не следует считать поддерживаемым только по наличию нескольких узлов. Такой режим допускается к публичному заявлению только для отдельно квалифицированного профиля с подтверждёнными failure/recovery, quorum и rollback semantics.

## Packaging и release evidence

Официальный Public Stable release set содержит и связывает с одной release identity:

- `control-center-0.31.0-linux-amd64.tar.gz`;
- SHA-256 sidecar и `SHA256SUMS`;
- source artifact;
- CycloneDX SBOM;
- `THIRD_PARTY_NOTICES.md`;
- qualification evidence;
- provenance;
- release manifest.

Контрольные суммы и release metadata должны проверяться перед установкой или обновлением. Уже опубликованные bytes релиза не заменяются новой сборкой под тем же version/tag.

## Product Stable и коммерческий запуск

0.31.0 является технически квалифицированным PUBLIC STABLE продукта. Commercial/legal clearance сохраняется как отдельный контур коммерческого запуска. Отсутствие подготовленных юридических документов не преобразуется в ложный commercial PASS, но не отменяет фактически опубликованный технический Stable.

До отдельного commercial clearance продукт и документация не должны заявлять неподтверждённые коммерческие, договорные или юридические гарантии. Лицензирование, Support Plans, billing, legal/privacy acceptance и связанные web-порталы вводятся только в пределах отдельно опубликованного и квалифицированного контура.

## Release integrity

Для 0.31.0 сохраняются следующие неизменяемые требования:

- false Success запрещён;
- stale/mismatched revision, approval, Job version или recovery evidence не принимается как current;
- migration checksum drift считается дефектом целостности;
- upgrade не должен сбрасывать пользовательский пароль, данные или настройки;
- high-risk security/privacy/recovery defect требует отдельного исправляющего release cycle;
- release artifacts/checksums/provenance/evidence должны оставаться согласованными с опубликованной release identity.

0.30.0 остаётся предыдущей стабильной версией и исходной точкой квалифицированного upgrade path к 0.31.0.
