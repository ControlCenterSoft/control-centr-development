# Control Center 0.43 — Architecture Freeze

Статус: **FROZEN FOUNDATION BASELINE FOR IMPLEMENTATION**

Дата фиксации: 2026-09-13

Этот документ фиксирует завершённую фундаментальную архитектуру milestone 0.43. После этой точки новые foundation-домены не добавляются в scope 1.0 без явного изменения roadmap. Разрешены уточнение contracts, security hardening, performance work, исправление ошибок и декомпозиция реализации, если они не создают параллельную архитектурную модель.

## 1. Канонический execution path

`Identity/RBAC → Desired State / Change → exact-revision Approval → durable Job → typed execution → Actual State → post-condition verification → Audit/Evidence → rollback/recovery`

Запрещены универсальный arbitrary shell как product API, скрытая mutation, last-writer-wins для управляемой конфигурации и false Success.

## 2. Основные invariants

- Single-node — полноценный поддерживаемый режим.
- HA заявляется только для квалифицированных profiles после failure/recovery tests.
- Unknown/Stale/Degraded не отображаются как Healthy.
- Backup без verified restore не является доказанной защитой.
- WAN+LAN не включает routing/NAT автоматически.
- Released SQL migrations immutable byte-for-byte.
- Approval всегда относится к exact revision/hash и не отменяет fresh execution preflight.
- Secrets в Plans/Jobs/Audit/logs/reports/support bundles не передаются в открытом виде.
- Provider объявляет только реально поддерживаемые capabilities; неподдерживаемая функция не имитируется generic shell.
- Greenfield и Brownfield — равноправные сценарии.
- Reconciliation предлагает объяснимый Change; он не выполняет скрытое исправление.

## 3. Frozen foundation domains

### 3.1 Infrastructure Intent & Solution Synthesis

First-class: InfrastructureIntent, IntentRevision, SolutionBlueprint, SolutionSynthesisRequest/Assessment, SolutionCandidate, ProviderBinding, Topology, ArchitectureConstraint, ArchitectureValidationResult, SolutionRevision, ExpansionAssessment/Plan.

Цепочка: `Intent → Blueprint → Synthesis → hard-constraint filtering → Architecture Validator → SolutionRevision → Deployment Plan`.

### 3.2 Managed Provider Framework

Уровни: OBSERVED / CONNECTED / MANAGED.

Provider lifecycle: discover, assess, adopt, install, configure, verify, operate, update, scale, cluster, migrate, drain, backup, restore, remove/decommission, expand-existing, create-new-instance/create-new-cluster где сертифицировано.

### 3.3 Bare-Metal Provisioning

BareMetalAsset, HardwareIdentity, BMCBinding, HardwareQualification, BootstrapProfile, ProvisioningPlan/Run/Evidence. Redfish — предпочтительная generic boundary; IPMI/vendor adapters — сертифицированные расширения.

### 3.4 Managed Network Fabric

NetworkFabric, NetworkDevice, FabricPort/Link, VLANSegment, LAG, FabricTopology, NetworkFabricPlan, CablingTask, FabricHealth/Evidence. External switch/fabric management отделён от host-side network safety 0.34–0.35.

### 3.5 Storage Infrastructure

StorageFabric/System/Node/Device/Pool/Class/Volume/Path, failure domains, redundancy/performance profiles, migration/rebalance/drain, health/evidence. RAID/replication/snapshot не равны backup.

### 3.6 IPAM / Addressing / Naming

AddressSpace, VRF/RoutingDomain, Prefix/Subnet, Pool, Reservation/Allocation, VLANBinding, NamingPolicy, Hostname/FQDN allocation, ServiceEndpoint, AddressingPlan/Evidence/Conflict. DNS/DHCP mutation остаётся отдельным provider lifecycle.

### 3.7 PKI / Certificate / Trust

TrustDomain, CA hierarchy, CertificateProfile/Identity/Binding, TrustBundle, Revocation, Validation, RotationPlan, TrustMigrationPlan, PKIHealth/Evidence. Internal, External и Public CA provider modes.

### 3.8 Secrets / Credentials / Service Identity

SecretRef/KeyRef, ServiceIdentity, CredentialProfile/Binding, Lease/Grant, BootstrapCredential, RotationPlan, health/evidence. Provider Runtime получает material только в exact capability/target/job scope.

### 3.9 Time / NTP / Clock Trust

TimeDomain, TimePolicy, TimeSourceGroup, TimeServer/ClientBinding, ClockObservation/Health/TrustStatus, TimePlan/Evidence. Synchronized time и trusted time различаются.

### 3.10 Artifact / Repository / Content Supply Chain

ArtifactIdentity = type/product/version + cryptographic digest. Admission, signature/provenance, quarantine, repository/cache/mirror, offline import. Approved plan связан с exact bytes.

### 3.11 Physical Infrastructure

Site/Building/Room/Rack/U-placement, assets, PDU/UPS/feed/circuit/PSU power paths, environmental/failure domains, PhysicalPlacementPlan/Health/Evidence. Логическая HA не доказывает физическую независимость.

### 3.12 Third-Party Licensing / Entitlement / Supportability

LicenseModel/Metric, Edition/Feature, Entitlement/Assignment/Consumption, Subscription/Support status, SupportabilityProfile, LicenseAssessment/Delta, CostEvidence. Техническая feasibility не равна licensing readiness.

### 3.13 Infrastructure BOM / Procurement Readiness

InfrastructureBOM/BOMRevision, Requirement/Item, InventoryMatch/Reservation, ResourceGap, EquivalentCandidate, FulfillmentEvidence, ReadinessAssessment/Gate. Сначала reuse подходящих existing assets, затем внешний gap.

### 3.14 Commissioning / Acceptance / Handover

CommissioningPlan/Run, AcceptanceProfile/Criterion/Test/Evidence, Finding/PunchList/ApprovedDeviation, AcceptanceDecision, AcceptedBaseline, HandoverPackage, OperationalReadinessAssessment. Deployment SUCCESS не равен ACCEPTED.

### 3.15 Operational Policy / SLO / Maintenance

OperationalPolicyRevision, CriticalityProfile, SLI/SLO, MaintenanceWindow, ChangeWindow, FreezeWindow, DisruptionBudget, ConcurrencyPolicy, ChangeEligibilityAssessment, EmergencyChangePolicy. Fresh preflight обязателен непосредственно перед execution.

### 3.16 Configuration Baseline / Drift / Compliance

ConfigurationBaseline/BaselineRevision, DesiredConfiguration, ActualConfigurationSnapshot, field-level ownership, DriftObservation/Assessment/Finding, CompliancePolicy/Rule/Assessment, ConfigurationException, ReconciliationPlan. Drift и compliance violation — разные понятия.

### 3.17 Vulnerability / Exposure / Patch Posture

SecurityAdvisory, VulnerabilityIdentity/Evidence/Finding, ExposureAssessment/Path, ExploitabilityEvidence, RiskAssessment, PatchCandidate, MitigationCandidate, RemediationPlan, SecurityException, PatchPostureSnapshot. CVE severity не заменяет contextual operational risk.

### 3.18 Asset Lifecycle / Warranty / EOL / Spares

AssetLifecycleProfile, WarrantyRecord, LifecycleMilestone/Finding, SparePool/Requirement/Asset, ReplacementAssessment/Plan, RetirementPlan, DisposalEvidence. EOL/Warranty expiry не выключают сервис автоматически, но влияют на risk/readiness.

### 3.19 External Dependency / WAN / Internet

ExternalDependency/Endpoint, ConnectivityPath, ExternalProviderService, DependencyFailureDomain/RedundancyPolicy, DependencyHealth/Evidence/SLO/Risk. Observed dependency не считается Managed dependency.

### 3.20 Data Governance / Retention / Privacy

DataClass/Policy/Location/Owner/Purpose, RetentionPolicy, EvidenceRetentionPolicy, DeletionPolicy/Request, LegalHold, DataExport, DataLifecycleEvidence. Удаление primary data не означает мгновенное исчезновение из immutable backups.

### 3.21 Integrations / ITSM / CMDB / Notifications / Webhooks

IntegrationDefinition/Binding/Capability/Identity, EventSubscription/WebhookSubscription, NotificationRoute, CMDBMapping, ExternalObjectBinding, TicketBinding, DeliveryAttempt, IntegrationHealth/Evidence. Внешняя интеграция не обходит RBAC/Change/Audit.

### 3.22 Cross-Domain Risk & Readiness

RiskFinding/Factor/Relation/Assessment/Snapshot/Treatment/Acceptance, ReadinessAssessment/Dimension, RiskEvidenceRef. Один correlated root cause не должен размножаться в несколько независимых рисков. Hard blocker не скрывается общим score.

### 3.23 Reference Architecture Qualification

ReferenceArchitectureProfile, QualificationScenario/Matrix/Run/Evidence, CertifiedTopologyProfile, SupportMatrix, FrozenArchitectureBaseline. Обязательны single-node, Greenfield, Brownfield, expansion, failure/recovery, security, operational, data/integration scenarios.

## 4. Frozen dependency order

`Intent → Blueprint → Synthesis → Architecture Validation → BOM/Readiness → Network/Bare Metal/Storage/Trust Foundations → Providers → Deployment → Verification → Commissioning → AcceptedBaseline → Operational Policy → Drift/Compliance/Security/Lifecycle → Reconciliation/Expansion`.

## 5. Reference qualification topology

Минимальный сложный профиль: 2 managed switches; 3 Proxmox VE nodes в независимых failure domains; distributed storage profile; PBS; monitoring; 2 directory/DNS nodes; независимые network/power paths; IPAM; trusted time; PKI; Secrets; Artifact repository. Support Gateway включается только в соответствующий support profile.

Отдельно квалифицируется полноценный single-node profile.

## 6. Freeze rule

После этого baseline новые foundation domains не входят в Control Center 1.0 без owner-approved roadmap change. Milestone 0.43 теперь должен двигаться через contracts → persistence → API → provider runtime → UI → qualification, а не через дальнейшее расширение фундаментальной архитектуры.
