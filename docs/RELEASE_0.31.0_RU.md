# Control Center 0.31.0 — Changes / Jobs operational workflow

Статус: **OFFICIAL SOURCE RELEASE после успешной exact-main qualification; готовность к PUBLIC STABLE определяется отдельным техническим promotion gate**.

Control Center 0.31.0 развивает текущий Public Stable 0.30.0 и вводит основной безопасный operational workflow вокруг типизированных Changes и Jobs. Публикация этой release identity допускается только после успешного CI точного итогового SHA и проверки технического Product Stable readiness. Коммерческий запуск и юридические документы ведутся отдельным контуром и не подменяются техническим release evidence.

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
- deterministic candidate packaging, SBOM, THIRD_PARTY_NOTICES, SHA256SUMS, provenance и release manifest;
- exact-bound release evidence aggregation.

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

Техническая qualification точного release SHA обязана подтвердить:

- clean install;
- supported upgrade с Public Stable 0.30.0;
- сохранность пользовательских данных и настроек;
- сохранность установленного пользователем пароля `admin` и first-login/password-change state;
- PostgreSQL 15/16/17/18 migration и adapter paths;
- restart/reconnect и race qualification;
- rollback через сохранённый pre-upgrade state и последующий forward recovery;
- reproducible exact candidate artifacts и checksums.

Обновление не сбрасывает установленный пароль администратора к `admin`. Первоначальный `admin/admin` используется только на чистой установке и требует обязательной смены при первом входе.

## Packaging и release evidence

Для точной release identity формируются и проверяются:

- `control-center-0.31.0-linux-amd64.tar.gz`;
- SHA-256 sidecar и `SHA256SUMS`;
- source artifact;
- CycloneDX SBOM;
- `THIRD_PARTY_NOTICES.md`;
- qualification evidence;
- provenance;
- release manifest;
- bounded exact-SHA readiness snapshot.

PASS не переносится между разными SHA. Любой source drift требует новой exact-head qualification.

## Product Stable и коммерческий запуск

PUBLIC STABLE продукта разрешён только если технический Product Stable evaluator подтверждает все обязательные product gates: operational E2E, packaging, clean install, supported upgrade, rollback/forward recovery, PostgreSQL restart/reconnect, security/privacy и release metadata.

Commercial/legal clearance сохраняется как отдельный контур коммерческого запуска. Отсутствие подготовленных юридических документов не преобразуется в ложный commercial PASS, но само по себе не блокирует публикацию технически квалифицированного продукта в PUBLIC STABLE. До отдельного commercial clearance продукт и документация не должны заявлять неподтверждённые коммерческие/юридические гарантии.

## Release stop conditions

0.31.0 не может быть опубликован как PUBLIC STABLE при любом из следующих условий:

- false Success или возможность представить непроверенный результат как успешный;
- stale/mismatched revision, approval, Job version или recovery evidence принимается как current;
- rollback/recovery path не доказан для risk-bearing operation;
- migration checksum drift;
- upgrade сбрасывает пользовательский пароль, данные или настройки;
- high-risk security/privacy/recovery defect;
- exact release SHA не прошёл обязательную qualification;
- отсутствуют или не совпадают обязательные release artifacts/checksums/provenance/evidence.

Официальный source release и PUBLIC STABLE promotion являются отдельными защищёнными этапами и должны сохранять одну и ту же квалифицированную release identity либо иметь явно проверенную promotion lineage.
