# Control Center

Control Center — самостоятельная платформа централизованного управления серверной и пользовательской ИТ‑инфраструктурой с типизированной, проверяемой и аудируемой моделью изменений.

## Текущий релизный статус

- Текущий официальный **PUBLIC STABLE — 0.31.1**. Он опубликован в `ControlCenterSoft/control-center-stable` как `v0.31.1` и является корректирующим patch-релизом без расширения feature scope.
- Исторический `v0.31.0` сохраняется неизменяемым. Его Linux-архив имеет известный package-shape defect: отсутствует обязательный `scripts/migrate.sh`. Для новых установок и обычных обновлений используется 0.31.1.
- Линия **0.32.0** остаётся текущей ближайшей COMMITTED release line: Health / Incidents / Audit / Reports. Наличие исходного кода, контрактов или отдельных UI-срезов не означает RC/Public Stable до завершения собственного release cycle.
- Параллельно начата ранняя foundation-реализация будущего milestone **0.43.0** в пределах уже замороженного архитектурного scope. Она не меняет ближайший release train 0.32 и не делает 0.43 доступным пользователю.

## Архитектурные принципы

Control Center разделяет Desired State и Actual State. Любое изменение должно выполняться через явный типизированный контракт, проверку прав и текущего состояния, управляемую операцию, post-condition verification и Audit. Для опасных действий обязателен заранее определённый rollback/recovery path.

Core включает общие платформенные функции: Identity/RBAC, Desired/Actual State, Changes/Jobs, Node/Role/Lifecycle, Network, Monitoring/Health, Audit, Backup/Recovery contracts, Capacity Planner foundation и системные API/security boundaries.

Market содержит устанавливаемые инфраструктурные возможности. Для каждого модуля обязательны identity, compatibility/dependencies, permissions/capabilities, network/storage requirements, capacity profile, backup/recovery semantics и явный lifecycle.

## Текущая опубликованная линия

Public Stable 0.31.1 сохраняет operational workflow Changes / Jobs линии 0.31 и исправляет package/install boundary 0.31.0. Официальный Linux AMD64 package содержит обязательный исполняемый `scripts/migrate.sh`; квалифицированы clean install и поддерживаемые переходы `0.30.0 → 0.31.1` и `0.31.0 → 0.31.1`. Неизвестное, устаревшее или неполное evidence не должно отображаться как подтверждённый успех.

После чистой установки создаётся локальная учётная запись `admin` с первоначальным паролем `admin`. Первый вход обязательно требует смены пароля; до смены обычная работа запрещена. При обновлении установленный пользователем пароль сохраняется и не сбрасывается к первоначальному значению.

## Целевая эксплуатационная модель

- полноценный single-node режим;
- multi-node/HA только для фактически поддержанных и проверенных ролей;
- maintenance, drain, replacement и decommission;
- безопасные staged network changes с connectivity verification и rollback;
- multi-NIC, WAN/LAN, VLAN/bonding/routing там, где это поддерживается;
- NAT/port-forwarding только при явном включении;
- backup вместе с проверяемым restore/recovery;
- Capacity Planner с safe capacity, bottleneck, forecast и what-if моделями;
- provider-specific migration/recovery для stateful workloads.

## Документация

Архитектурные требования, продуктовая дорожная карта и каталог требований находятся в `ARCHITECTURE.md`, `ROADMAP.md` и `docs/REQUIREMENTS_RU.md`. Пользовательская документация должна описывать только фактически опубликованные возможности и отдельно обозначать целевые/кандидатные функции.

Публичная продуктовая документация не раскрывает внутренние процессы разработки и сборки, служебную инфраструктуру, внутренние адреса, секреты, ключи, внутренние рабочие ветки/репозитории, персональные данные или иные сведения, не требующиеся пользователю и администратору продукта.
