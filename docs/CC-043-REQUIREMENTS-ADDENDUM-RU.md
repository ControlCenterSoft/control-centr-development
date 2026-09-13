# Control Center 0.43 — дополнение к требованиям

Статус: **NORMATIVE ADDENDUM FOR 0.43 IMPLEMENTATION**

Это дополнение уточняет обязательный scope 0.43 после Architecture Freeze и читается совместно с `docs/REQUIREMENTS_RU.md`, `ARCHITECTURE.md`, `ROADMAP.md`, `docs/CC-043-ARCHITECTURE-FREEZE-RU.md` и `docs/CC-043-IMPLEMENTATION-PLAN-RU.md`.

## 1. Пользовательский результат 0.43

Оператор должен иметь возможность:

- описать Infrastructure Intent;
- выбрать/получить Blueprint и несколько объяснимых Solution Candidates;
- увидеть reuse существующей инфраструктуры и материальные gaps;
- проверить topology, capacity, physical/power, network/storage, recovery, security, licensing/supportability и readiness;
- получить Proposed SolutionRevision и exact DeploymentPlan;
- принять Brownfield topology без обязательной переустановки;
- подготовить expansion существующего Solution;
- видеть Providers с фактическим management level и capabilities;
- после deployment пройти commissioning/acceptance и получить AcceptedBaseline;
- в operating-state видеть policy, drift/compliance, posture, lifecycle, external-dependency и cross-domain risk findings.

## 2. Безопасность

- Все state changes используют существующие Identity/RBAC → Change/Approval → Job → verification → Audit/recovery boundaries.
- Нет универсального arbitrary execution API.
- Provider Runtime получает только exact typed capability/context.
- Секретные значения не попадают в Plans/Jobs/Audit/logs/reports/support bundles.
- Unsupported/Unknown/Stale states работают fail-closed для mandatory gates.

## 3. Данные

- Migrations 0001–0013 immutable.
- 0.43 использует только additive migrations 0014+.
- Large artifacts, backups и heavy telemetry не хранятся как unbounded blobs в primary PostgreSQL.
- Versioned objects имеют scope/revision semantics; evidence имеет freshness/confidence where applicable.

## 4. Provider boundary

0.43 реализует Provider Contract v1 и reference qualification adapters. Это не означает автоматическую публикацию production Market capability для Directory, Monitoring, Backup и других последующих milestones. Public support claims появляются только после соответствующей provider qualification/release.

## 5. Architecture Freeze

Новый foundation-domain после этого документа требует explicit roadmap change. Разрешены уточнения contracts, implementation details, hardening, bug fixes и performance work внутри frozen boundaries.

## 6. Definition of Done

0.43 не считается завершённой при наличии false Success, неподтверждённого upgrade/data-preservation path, несоответствия UI фактическим capabilities, обходного mutation path или reference qualification, не покрывающей Greenfield, Brownfield и expansion.