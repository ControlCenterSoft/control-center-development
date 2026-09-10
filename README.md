# Control Center

Control Center — централизованная платформа управления инфраструктурой с типизированной, проверяемой и аудируемой моделью исполнения.

Текущий опубликованный исходный релиз: **0.17.0**. Отдельный стабильный бинарный канал распространения на момент этой редакции подтверждён до **0.3.1**; опубликованный исходный релиз и готовый бинарный пакет не следует смешивать.

## Документация

- [`ARCHITECTURE.md`](ARCHITECTURE.md) — продуктовая архитектура и обязательные инварианты;
- [`ROADMAP.md`](ROADMAP.md) — опубликованная и ближайшая продуктовая линия;
- [`docs/REQUIREMENTS_RU.md`](docs/REQUIREMENTS_RU.md) — каталог принятых требований.

## Возможности опубликованной линии

Core включает HTTP/JSON API и health/readiness, локальную identity/session модель, deny-by-default RBAC, аудит, PostgreSQL-backed durable state, immutable configuration revisions, policy/risk/approval-aware Changes, durable Jobs, типизированные действия, resource state/health, Agent enrollment/heartbeat foundations и общие Inventory/Market/PXE/Automation/Domain/Integration contracts.

В опубликованной линии Capacity Planner последовательно добавлены forecast/what-if, Placement Advice, Bottleneck Report, Capacity Horizon, Calibration, Forecast Correction, Calibration Trend, benchmark-backed Workload Profile, bounded nonlinear Workload Curve, Workload Curve Efficiency и в **0.17.0** — Workload Scale Scenario/Scale Options. Эти Capacity-функции являются advisory-only: они не разрешают автоматический placement, resize, migration, rebalance или иное изменение production-инфраструктуры.

## Архитектурная модель

Control Center поддерживает самостоятельный single-node профиль и целевую multi-node/HA модель с разделением ролей, Desired State и Actual State, управляемым lifecycle узлов, recovery, Network Management и Capacity Planner. Наличие архитектурного контракта не означает, что конкретный HA/failover профиль уже сертифицирован для опубликованной версии; поддержка считается доступной только там, где она явно подтверждена документацией соответствующего релиза.

Network Management является частью Core: multi-NIC, WAN/LAN и другие зоны, routing, VLAN/bonding там, где это поддерживается, DNS/NTP, firewall и staged changes с проверкой связности и rollback. NAT/port-forwarding включаются только явно; наличие WAN+LAN само по себе не превращает узел в шлюз.

Lifecycle охватывает enrollment, maintenance, drain, replacement/decommission и recovery. Для опасных операций должны быть заранее определены риск, проверка результата и rollback/recovery path. Stateful workloads требуют provider-specific migration/recovery semantics.

Market содержит устанавливаемые инфраструктурные возможности и не смешивается с Core. Для модуля должны быть определены compatibility/dependencies, permissions, network/storage requirements, capacity profile и lifecycle Install → Configure → Health → Update → Migrate/Drain → Backup → Restore → Remove; failover добавляется только когда он фактически поддерживается provider.

## Первый вход

Для опубликованной исходной линии **0.6.0–0.17.0** на чистой установке создаётся локальный пользователь `admin` с первоначальным паролем `admin`. Первая сессия допускает только обязательные действия, необходимые для смены первоначального пароля; до смены обычная работа запрещена. При обновлении существующий пользовательский пароль не сбрасывается и не заменяется первоначальным credential.

Отдельный бинарный stable-выпуск **0.3.1** использует более раннюю bootstrap-модель аутентификации; при эксплуатации этого бинарного выпуска следует применять его собственную release-документацию.

## Локальная сборка

```bash
make build
./bin/control-center
```
