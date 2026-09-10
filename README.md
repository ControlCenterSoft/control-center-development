# Control Center

Control Center — самостоятельная платформа централизованного управления серверной и пользовательской ИТ-инфраструктурой с типизированной, проверяемой и аудируемой моделью выполнения операций.

## Текущий статус

Последний опубликованный исходный релиз Control Center — **0.13.0**.

Отдельный стабильный бинарный канал распространения на текущий момент подтверждён до **0.3.1**. Публикация исходного релиза и наличие готового бинарного дистрибутива — разные границы: для установки следует использовать только явно опубликованный и проверяемый артефакт соответствующего канала.

Функции более новых версий считаются предварительными до официальной публикации соответствующего релиза.

## Возможности опубликованной исходной линии

Накопительная линия Control Center включает:

- HTTP/JSON API, health и readiness;
- локальную identity/session модель и deny-by-default RBAC;
- append-oriented audit;
- PostgreSQL-backed durable state;
- immutable configuration revisions;
- Changes с учётом политики, риска и approval;
- durable Jobs с leases, retries и idempotency;
- allowlisted typed actions вместо произвольного удалённого shell;
- состояние и health управляемых ресурсов;
- основы enrollment/heartbeat для узлов;
- основы Inventory, Market, PXE, Automation, Domain и Integration;
- Site-модель и безопасную автономную работу в пределах делегированного scope;
- Network Foundation с явными зонами, Edge Gateway и staged network changes;
- Capacity Intelligence: forecast/what-if, Placement Advice, Bottleneck Report, Capacity Horizon, Capacity Calibration, Forecast Correction и Calibration Trend.

Capacity Intelligence в опубликованной линии остаётся **advisory-only**: аналитический результат сам по себе не разрешает placement, migration, resize, сетевые изменения или иные инфраструктурные mutations. Forecast Correction не использует калибровочный коэффициент меньше `1` для автоматического снижения planning workload, а Calibration Trend только оценивает устойчивость и drift calibration evidence.

## Архитектурные принципы

Control Center разделяет Desired State и Actual State. Канонический путь изменения состояния:

`Запрос → Валидация → Авторизация → План → Change → Job → типизированное действие → Проверка → Actual State → Audit`

Опасная операция должна иметь понятные risk, preflight, verification и recovery/rollback semantics. Успешное завершение команды или Job не считается доказательством результата без проверки фактического состояния.

## Single-node и multi-node/HA

Single-node является самостоятельным способом использования Control Center и не требует второго сервера.

Целевая архитектура предусматривает multi-node, распределение ролей, maintenance/drain, replacement/decommission, восстановление и HA. Конкретный HA-профиль считается поддерживаемым только после его реализации, проверки quorum/fencing/failover/recovery и официальной публикации соответствующей версии. Наличие нескольких узлов или архитектурных контрактов само по себе не является доказательством production HA.

## Сеть

Network Management является частью Core. Поддерживаемая модель предусматривает multi-NIC, WAN/LAN и другие зоны, VLAN/bonding там, где это реализовано, routing, DNS/NTP и firewall policy.

Наличие WAN+LAN не включает routing, forwarding, NAT или port-forwarding автоматически. Такие возможности требуют отдельного явного разрешения и безопасного Change. Рискованные сетевые изменения должны применяться staged с проверкой связности и rollback.

## Core и Market

**Core** содержит обязательные платформенные функции: Identity/RBAC, Desired/Actual State, Changes/Jobs, Node/Role/Lifecycle, Network, Monitoring/Health, Audit, Backup/Recovery contracts, Capacity Planner foundation и общие security boundaries.

**Market** содержит устанавливаемые инфраструктурные возможности. Модуль должен иметь явную identity, compatibility/dependency metadata, permissions/capabilities, network/storage requirements, capacity profile и lifecycle. Зафиксированные направления включают Directory Services с поддерживаемыми Samba AD/FreeIPA providers, DNS/DHCP, PXE Windows/Linux, Software Automation Windows/Linux, Inventory/Compliance, File Services, Monitoring и Backup.

## Первый вход

Для опубликованной исходной линии начиная с 0.6.0 после чистой установки создаётся локальный пользователь `admin` с первоначальным паролем `admin`. При первом входе пароль необходимо сменить; до смены обычная работа с системой запрещена. При обновлении существующий пользовательский пароль `admin` сохраняется и не сбрасывается к первоначальному значению.

Отдельный бинарный выпуск 0.3.1 относится к более ранней bootstrap-модели аутентификации; при эксплуатации этого бинарного выпуска необходимо следовать его собственной release-документации.

## Документация

- [`ARCHITECTURE.md`](ARCHITECTURE.md) — целевая архитектура и обязательные инварианты;
- [`ROADMAP.md`](ROADMAP.md) — опубликованные этапы и дальнейшее развитие;
- [`docs/REQUIREMENTS_RU.md`](docs/REQUIREMENTS_RU.md) — каталог принятых требований;
- [`docs/RELEASE_0.13.0_RU.md`](docs/RELEASE_0.13.0_RU.md) — состав текущего опубликованного исходного релиза.

## Локальная сборка из исходного кода

```bash
make build
./bin/control-center
```

Готовый production-дистрибутив следует брать только из явно опубликованного бинарного канала и проверять согласно сопровождающей его release-документации.
