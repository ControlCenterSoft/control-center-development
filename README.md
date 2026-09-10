# Control Center

Control Center — самостоятельная централизованная платформа управления инфраструктурой с типизированной, проверяемой и аудируемой моделью исполнения.

Текущая версия исходного кода: **0.20.0**. Последний подтверждённый официальный GitHub Release на момент этой синхронизации: **0.19.0**. Отдельный стабильный бинарный канал опубликован как **0.3.1**; исходная и бинарная линии не следует смешивать.

## Базовые возможности

- HTTP/JSON API и health/readiness endpoints;
- локальная Identity/Session модель и deny-by-default RBAC;
- обязательная смена первоначального пароля `admin` после чистой установки;
- PostgreSQL-backed durable state;
- Desired State и Actual State как раздельные модели состояния;
- Changes/Jobs, идемпотентность, retries и audit;
- Agent enrollment/heartbeat и inventory foundations;
- Node/Role/Lifecycle contracts;
- Network Management с multi-NIC, WAN/LAN, VLAN/bonding, routing, DNS/NTP, firewall и безопасными staged changes;
- Capacity Planner / Placement Advisor с детерминированными advisory-only расчётами;
- Market contracts для устанавливаемых инфраструктурных модулей;
- Backup/Recovery contracts и безопасные границы опасных операций;
- single-node как самостоятельный режим и целевая multi-node/HA архитектура.

## Core и Market

**Core** содержит обязательные функции платформы: Identity/RBAC, Desired/Actual State, Changes/Jobs, Node/Role/Lifecycle, Network, Monitoring/Health, Audit, Backup/Recovery contracts, Capacity Planner foundation и системные API.

**Market** содержит устанавливаемые инфраструктурные возможности с явными compatibility/dependency, permissions, network/storage, capacity и lifecycle metadata. К направлениям Market относятся Directory Services, DNS/DHCP, PXE для Windows/Linux, Software Automation для Windows/Linux, Inventory/Compliance, File Services, Monitoring и другие поддерживаемые инфраструктурные providers.

## Single-node и HA

Single-node является полноценным поддерживаемым способом использования Control Center. Multi-node/HA вводится только для возможностей, для которых опубликованы и проверены соответствующие failure/recovery contracts. Наличие архитектурного контракта само по себе не означает сертифицированный production failover.

## Desired / Actual State

Изменение инфраструктуры проходит контролируемый путь:

`Запрос → Валидация → Авторизация → План → Change → Job → типизированное действие → Проверка → Actual State → Audit`

Desired State не подменяется текущим Actual State. Для опасных операций обязательно определяются риск, preflight, проверка результата и rollback/recovery path.

## Capacity Planner

Опубликованная Capacity-линия остаётся advisory-only. Она использует измеренные workload/capacity evidence и детерминированные контракты для forecast, what-if, placement advice, bottleneck/capacity horizon, calibration, workload profiles/curves, scale scenarios, safety margin и resource headroom. Такие расчёты не дают разрешения на автоматическую production mutation без отдельного policy/approval слоя.

## Аутентификация

После чистой установки создаётся локальный пользователь `admin` с первоначальным паролем `admin`. При первом входе пароль необходимо сменить; до смены обычная работа запрещена. При обновлении существующий пароль пользователя не сбрасывается к первоначальному значению.

## Локальная проверка и сборка

```bash
make ci
make build
./bin/control-center
```

Подробные архитектурные требования, release notes и эксплуатационные инструкции находятся в документации продукта.
