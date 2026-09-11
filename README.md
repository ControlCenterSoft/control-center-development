# Control Center

Control Center — самостоятельная платформа централизованного управления серверной и пользовательской ИТ‑инфраструктурой с типизированной, проверяемой и аудируемой моделью изменений.

## Текущий релизный статус

- Последний официально опубликованный canonical/source release: **0.26.0**.
- Полноценный **PUBLIC STABLE RELEASE 0.26.0** опубликован в [`ControlCenterSoft/control-center-stable`](https://github.com/ControlCenterSoft/control-center-stable): актуальны default branch, tag `v0.26.0`, официальный GitHub Release и предусмотренные artifacts/checksums/qualification/release manifests/provenance.
- **0.25.0** сохраняется как предыдущая стабильная версия.
- Более новые development-возможности не должны описываться как доступные пользователю до официальной публикации соответствующей release identity.

## Архитектурные принципы

Control Center разделяет Desired State и Actual State. Любое изменение должно выполняться через явный типизированный контракт, проверку прав и текущего состояния, управляемую операцию, post-condition verification и Audit. Для опасных действий обязателен заранее определённый rollback/recovery path.

Core включает общие платформенные функции: Identity/RBAC, Desired/Actual State, Changes/Jobs, Node/Role/Lifecycle, Network, Monitoring/Health, Audit, Backup/Recovery contracts, Capacity Planner foundation и системные API/security boundaries.

Market содержит устанавливаемые инфраструктурные возможности. Для каждого модуля обязательны identity, compatibility/dependencies, permissions/capabilities, network/storage requirements, capacity profile, backup/recovery semantics и явный lifecycle.

## Текущая опубликованная линия

Public Stable 0.26.0 включает безопасную локальную аутентификацию и сессии, RBAC self-introspection, Audit, lifecycle/recovery contracts, advisory Capacity Intelligence, bounded-защиту локального входа от brute force/credential spraying, permission-gated bounded read-only доступ к Audit events и permission-gated read-only проверку целостности append-only Audit-цепочки. Audit Integrity работает fail-closed; для поддерживаемых legacy PostgreSQL-схем используется новая compatibility migration без изменения уже опубликованных migration-файлов.

Capacity-рекомендации не предоставляют автоматическое право на изменение инфраструктуры.

Для чистой установки Public Stable 0.26.0 создаётся локальная учётная запись `admin` с первоначальным паролем `admin`. При первом входе пароль необходимо сменить; до смены обычная работа запрещена. При обновлении установленный пользователем пароль сохраняется и не сбрасывается к первоначальному значению.

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

Продуктовая документация не должна содержать внутреннюю инфраструктуру разработки, runner-инфраструктуру, служебные адреса, секреты, ключи, персональные данные, названия внутренних AI/reviewer-процессов или сведения, не требующиеся пользователю и администратору продукта.
