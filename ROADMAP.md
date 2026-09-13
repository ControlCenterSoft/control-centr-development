# Control Center — продуктовая дорожная карта и критерии готовности

Статус: **CURRENT / SOURCE OF TRUTH FOR DEVELOPMENT SEQUENCE**

## 1. Текущий релизный статус

Текущий опубликованный Public Stable — **0.31.0**. Release train 0.31 завершён, однако у опубликованного Linux archive 0.31.0 известен package-shape defect: отсутствует обязательный `scripts/migrate.sh`. Immutable tag/release/assets 0.31.0 не переписываются. Corrective patch identity — **0.31.1**; её публикация требует отдельной qualification и promotion.

Линия **0.32.0** — текущая COMMITTED development line: Health / Incidents / Audit / Reports. Наличие merged code или contracts не означает Public Stable до прохождения собственного release cycle.

## 2. Неизменяемые правила

Канонический путь изменения состояния:

`Identity/RBAC → Desired State / Change → exact-revision Approval → durable Job → typed execution → Actual State → post-condition verification → Audit/Evidence → rollback/recovery`

Обязательные invariants:

- arbitrary shell/exec не является универсальным product API;
- Unknown/Stale/Degraded не отображаются как Healthy;
- false Success является release blocker;
- WAN+LAN не включает routing/NAT автоматически;
- backup без verified restore не считается доказанной защитой;
- HA без failure/recovery qualification не считается поддержанным;
- released SQL migrations immutable byte-for-byte;
- stateful workload перемещается только через provider-specific migration/recovery semantics.

## 3. Аутентификация после чистой установки

Чистая установка создаёт локального пользователя `admin` с первоначальным паролем `admin`. Первый вход обязательно требует смены пароля; до смены обычная работа запрещена. Обновление сохраняет установленный пользователем пароль и никогда не сбрасывает его обратно к `admin/admin`.

## 4. Ближайшая линия 0.32–0.42

- **0.32.0 — COMMITTED.** Health / Incidents / Audit / Reports.
- **0.33.0 — PLANNED.** Identity / RBAC / Session / Security Settings UI.
- **0.34.0 — PLANNED.** Managed Network Planning UI.
- **0.35.0 — PLANNED.** Managed Network Apply / Verify / Rollback.
- **0.36.0 — PLANNED.** Node/Agent Enrollment, Trust и Support Gateway / Support Bundle Server.
- **0.37.0 — PLANNED.** Maintenance / Drain / Replacement / Decommission.
- **0.38.0 — PLANNED.** Role Placement + Capacity integration.
- **0.39.0 — PLANNED.** Recovery Points / Backup Repository foundation.
- **0.40.0 — PLANNED.** PostgreSQL Recovery / PITR / restore verification.
- **0.41.0 — PLANNED.** Controller Membership / Quorum / DCS.
- **0.42.0 — PLANNED.** HA / Controlled Switchover / Failover.

Recovery foundation предшествует HA. Planning UI предшествует risk-bearing network execution. Node enrollment предшествует lifecycle automation.

## 5. Milestone 0.43 — Architecture Freeze

**0.43.0 — PLANNED. Managed Provider Framework + Infrastructure Solutions Foundation + Intent / Synthesis / Expansion + Market Platform v2.**

Фундаментальная архитектура 0.43 заморожена в:

- [`docs/CC-043-ARCHITECTURE-FREEZE-RU.md`](docs/CC-043-ARCHITECTURE-FREEZE-RU.md)
- [`docs/CC-043-IMPLEMENTATION-PLAN-RU.md`](docs/CC-043-IMPLEMENTATION-PLAN-RU.md)

После Architecture Freeze новые foundation-domains не добавляются в scope 1.0 без явного roadmap change. Реализация 0.43 должна двигаться через contracts → persistence → API → Provider Runtime → Solution Orchestrator → Product Web UI → reference qualification.

Frozen foundation включает:

- Infrastructure Intent / Requirements;
- Solution Catalog / Blueprint Library;
- Solution Synthesis / Architecture Validator / Expansion Planner;
- Managed Provider Framework / Provider Contract v1;
- Bare Metal Provisioning;
- Managed Network Fabric;
- Storage Infrastructure;
- IPAM / Addressing / Naming;
- PKI / Certificate / Trust;
- Secrets / Credentials / Service Identity;
- Time / NTP / Clock Trust;
- Artifact / Repository / Content Supply Chain;
- Physical Infrastructure / Rack / Power / Failure Domains;
- Third-Party Licensing / Entitlement / Supportability;
- Infrastructure BOM / Procurement Readiness;
- Commissioning / Acceptance / Handover;
- Operational Policy / SLO / Maintenance & Change Windows;
- Configuration Baseline / Drift / Compliance;
- Vulnerability / Exposure / Patch Posture;
- Asset Lifecycle / Warranty / EOL / Spares;
- External Dependency / WAN / Internet dependencies;
- Data Governance / Retention / Privacy;
- Integrations / ITSM / CMDB / Notifications / Webhooks;
- Cross-Domain Risk & Readiness;
- Reference Architecture Qualification.

Greenfield и Brownfield являются равноправными сценариями. Expansion поддерживает как expand-existing, так и create-new-instance/create-new-cluster в пределах certified provider capabilities.

## 6. Market milestones 0.44–0.55

- **0.44** Directory Services providers: Samba AD / FreeIPA.
- **0.45** DNS / DHCP.
- **0.46** PXE Deployment Windows / Linux.
- **0.47** Software Automation Windows / Linux.
- **0.48** IT Asset Inventory.
- **0.49** Software Inventory & Compliance.
- **0.50** File Services.
- **0.51** Monitoring provider.
- **0.52** Backup providers.
- **0.53** Mail & Groupware.
- **0.54** 1C:Enterprise Server.
- **0.55** Secure Web Gateway / Corporate Proxy.

Конкретный provider не может объявлять capability, отсутствующую в его qualified Provider Contract.

## 7. Capacity / policy-driven operations 0.56–0.59

- **0.56** Capacity Intelligence v2.
- **0.57** Policy-driven Placement.
- **0.58** Controlled Automatic Rebalance.
- **0.59** Bounded Automatic Recovery.

Автоматизация разрешена только в явно заданной policy boundary и не заменяет Change/Job/Audit.

## 8. Mobile 0.60–0.61

- **0.60** Mobile v1 read-focused.
- **0.61** bounded mobile actions with server-side revalidation.

Mobile не создаёт обходных административных API.

## 9. Hardening / 1.0

- **0.62** Accessibility / Localization / Security / Performance hardening.
- **0.63** Install / Upgrade / Rollback / Migration certification.
- **0.64** Scale certification.
- **0.65** HA/DR disaster drills.
- **0.66** Integrated Production Readiness.
- **0.90** Feature Freeze.
- **0.95** Release Candidate.
- **1.0.0** Public Stable target for the known scope.

## 10. Definition of Done capability

Capability готова только при наличии:

1. object/data/API contract;
2. RBAC permissions/scopes;
3. Desired/Actual semantics для mutations;
4. failure/recovery model;
5. validation, stale-state protection и idempotency;
6. health/observability/Audit semantics;
7. positive/failure/security tests;
8. upgrade/migration path;
9. backup/restore semantics для stateful data;
10. user/operations documentation;
11. фактического соответствия реализации заявленному поведению.

Для risk-bearing operation дополнительно обязательны exact target, preview/diff, blast radius, preflight, approval policy, durable Job, post-condition verification и recovery path.

## 11. Release rule

Каждый начатый release train обязан завершаться официальным Public Stable release. COMMITTED/RC/SOURCE RELEASE — промежуточные состояния. Коммерческие/юридические материалы могут идти параллельно и не должны удерживать технически готовый Public Stable, если неподтверждённые commercial capabilities выключены и не заявляются. Security, upgrade, rollback, recovery, data-preservation и false-success gates обходить нельзя.

## 12. Product boundary

Control Center — самостоятельный infrastructure control plane. Product documentation не раскрывает внутреннюю development/CI methodology, внутренние адреса, repository mechanics, secrets или иные служебные данные.