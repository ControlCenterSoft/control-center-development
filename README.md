# Control Center

Control Center — самостоятельная платформа централизованного управления серверной и пользовательской ИТ‑инфраструктурой с типизированной, проверяемой и аудируемой моделью изменений.

## Текущий релизный статус

- Последний официально опубликованный исходный релиз в этом репозитории: **0.24.0**.
- Отдельный стабильный бинарный канал подтверждён до **0.3.1** и опубликован в [`ControlCenterSoft/control-center-stable`](https://github.com/ControlCenterSoft/control-center-stable).
- Возможности, появившиеся после stable-binary 0.3.1, нельзя считать доступными в этом бинарном канале до отдельной публикации соответствующей стабильной сборки.
- Следующая версия считается кандидатной до завершения её qualification и официальной публикации.

## Архитектурные принципы

Control Center разделяет Desired State и Actual State. Любое изменение должно выполняться через явный типизированный контракт, проверку прав и текущего состояния, управляемую операцию, post-condition verification и Audit. Для опасных действий обязателен заранее определённый rollback/recovery path.

Core включает общие платформенные функции: Identity/RBAC, Desired/Actual State, Changes/Jobs, Node/Role/Lifecycle, Network, Monitoring/Health, Audit, Backup/Recovery contracts, Capacity Planner foundation и системные API/security boundaries.

Market содержит устанавливаемые инфраструктурные возможности. Для каждого модуля обязательны identity, compatibility/dependencies, permissions/capabilities, network/storage requirements, capacity profile, backup/recovery semantics и явный lifecycle.

## Текущая опубликованная линия

Официальная исходная линия до 0.24.0 включает развитие безопасной локальной аутентификации и сессий, RBAC self-introspection, Audit, lifecycle/recovery contracts и advisory Capacity Intelligence. Capacity-рекомендации не предоставляют автоматическое право на изменение инфраструктуры.

Для чистой установки опубликованной исходной линии 0.6.0–0.24.0 действует bootstrap-учётная запись `admin` с первоначальным паролем `admin`. При первом входе пароль необходимо сменить; до смены обычная работа запрещена. При обновлении установленный пользователем пароль сохраняется и не сбрасывается к первоначальному значению.

Stable-binary 0.3.1 использует более раннюю bootstrap-модель. Правила `admin/admin` исходной линии 0.6.0–0.24.0 не следует автоматически применять к этому бинарному каналу; для 0.3.1 используется его собственная документация и bootstrap credential.

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

Продуктовая документация не должна содержать внутреннюю инфраструктуру разработки, служебные адреса, секреты, ключи, персональные данные или сведения, не требующиеся пользователю и администратору продукта.
