# Дорожная карта Control Center

## Текущая точка

Последний опубликованный исходный релиз Control Center — **0.24.0**. Отдельный стабильный бинарный канал подтверждён до **0.3.1**. Публикация исходного релиза не означает автоматическую публикацию бинарного пакета той же версии.

Опубликованная линия 0.6.0–0.24.0 последовательно развивает Core, сетевые контракты, Capacity Intelligence и безопасность локальной аутентификации. Версия 0.23.0 добавила безопасный read-only RBAC self-introspection для текущей identity. Версия 0.24.0 добавила bounded process-local защиту локального входа от brute force и credential spraying с anti-enumeration поведением.

Следующая подготовленная версия **0.25.0** предназначена для ограниченного permission-gated чтения событий Audit. Она не считается опубликованной возможностью до официального выпуска.

## 1. Core

Control Center Core развивается как единая платформа управления состоянием инфраструктуры. К обязательным направлениям относятся:

- Identity, локальная аутентификация и deny-by-default RBAC;
- Session Security и Audit;
- Desired State / Actual State;
- типизированные Changes и durable Jobs;
- Node/Role/Lifecycle;
- Monitoring/Health;
- Network Management;
- Backup/Recovery contracts;
- Capacity Planner;
- системные API и общие security boundaries.

Общие платформенные функции не должны дублироваться внутри Market-модулей отдельными несовместимыми реализациями.

## 2. Single-node, multi-node и HA

Single-node остаётся самостоятельным поддерживаемым профилем эксплуатации. Он не является HA: потеря единственного узла приводит к недоступности management plane до восстановления.

Целевая multi-node/HA модель предусматривает распределение ролей, управляемое membership, quorum там, где он необходим, fencing/split-brain protection, контролируемый switchover, совместимые профили PostgreSQL HA и проверяемое восстановление. Наличие архитектурных контрактов не следует трактовать как доказанный production failover. HA-профиль считается поддерживаемым только после отдельной публикации и проверки отказов и восстановления.

## 3. Desired State / Actual State

Desired State фиксирует требуемое состояние, Actual State — фактически наблюдаемое. Изменения должны иметь явное намерение, проверяемые preconditions и итоговую сверку факта с ожидаемым состоянием.

Для действий, способных повлиять на доступность, данные или сеть, должны быть определены риск, blast radius, способ проверки результата и recovery/rollback path. Система не должна выдавать Success, если фактическое состояние не подтверждает результат операции.

## 4. Lifecycle узлов и сервисов

Целевая модель включает enrollment, active, maintenance, drain, replacement и decommission/remove. Lifecycle должен учитывать зависимости сервисов, доступную capacity и состояние данных.

Stateless workloads могут перемещаться только после проверки совместимости и ресурсов. Stateful workloads требуют provider-specific migration/recovery adapter и не должны переноситься как обычный stateless сервис.

## 5. Recovery

Recovery является самостоятельной продуктовой подсистемой. Целевая модель включает Recovery Points, проверяемые backup metadata, изолированный restore, PostgreSQL backup/PITR для поддерживаемых профилей, object-level recovery для применимых объектов, измеряемые RPO/RTO и регулярные restore drills.

Backup без проверенного restore не считается достаточным доказательством готовности к восстановлению.

## 6. Network Management

Сеть является частью Core. Целевая функциональность включает:

- multi-NIC;
- назначаемые WAN/LAN/MANAGEMENT и другие зоны;
- VLAN и bonding там, где поддерживается платформа;
- routing;
- DNS/NTP;
- firewall policy;
- NAT/port-forwarding только при явном включении;
- staged network changes;
- connectivity verification;
- автоматический rollback при потере управляемости.

Наличие WAN и LAN на одном сервере не должно автоматически превращать Control Center в маршрутизатор или edge gateway.

## 7. Capacity Planner

Capacity Planner развивается от наблюдения к проверяемым рекомендациям. Опубликованная линия уже включает forecast/what-if, placement advice, bottleneck analysis, capacity horizon, calibration/trends, workload profiles/curves и дополнительные safety/freshness/resource-reuse ограничения.

Дальнейшее развитие предусматривает использование фактической telemetry, прогноз исчерпания резерва, DB/storage/network bottleneck analysis, what-if для устройств и Market workloads, рекомендации по добавлению или перемещению ролей и confidence/failure reserve.

Текущие Capacity-возможности остаются **advisory-only**: рекомендация сама по себе не даёт права автоматически изменять инфраструктуру.

## 8. Market

Market содержит устанавливаемые инфраструктурные возможности и остаётся отделённым от Core. Для каждого модуля должны быть определены compatibility/dependencies, permissions, network/storage requirements, capacity profile, health и recovery semantics.

Основные направления:

- Directory Services, включая Samba AD и FreeIPA для поддерживаемых сценариев;
- DNS/DHCP;
- PXE Windows/Linux;
- Software Automation Windows/Linux;
- Inventory/Compliance;
- File Services;
- Monitoring;
- Backup и другие providers через единый module contract.

Целевой lifecycle модуля: `Install → Configure → Health → Update → Migrate/Drain → Backup → Restore → Remove`. Failover добавляется только для provider, где он фактически поддержан и проверен.

## 9. Security и authentication

После чистой установки опубликованной исходной линии создаётся локальный `admin` с первоначальным паролем `admin`; до обязательной смены этого пароля обычная работа запрещена. Обновление не должно сбрасывать уже заданный пользовательский пароль.

Опубликованные 0.22.0–0.24.0 последовательно добавили Session Security, RBAC self-introspection и bounded process-local login abuse protection. Следующий пакет 0.25.0 развивает read-only Audit access с permission gating, bounded pagination, redaction и fail-closed evidence semantics, но до официальной публикации остаётся кандидатной возможностью.

## 10. Следующие крупные направления

Дальнейшая продуктовая работа сосредоточена на следующих направлениях без изменения основных архитектурных границ:

- зрелый multi-node lifecycle и безопасное распределение ролей;
- сертифицированные HA/DR профили;
- полноценный Recovery Manager и восстановление объектов;
- безопасное first-class управление сетью;
- Market Platform с единым lifecycle и provider contracts;
- дальнейшая калибровка Capacity Planner;
- policy-driven operations только после накопления достаточных доказательств безопасности и восстановления.

Автоматическое изменение stateful workloads без provider-specific migration/recovery adapter запрещено.
