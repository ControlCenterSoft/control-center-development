# Control Center

Control Center — самостоятельная платформа централизованного управления инфраструктурой с проверяемой моделью Desired State / Actual State, ролевым доступом, аудитом и безопасным жизненным циклом изменений.

## Текущий релизный статус

- опубликованный исходный релиз: **0.24.0**;
- отдельный стабильный бинарный канал: **0.3.1**.

Эти линии различаются: публикация исходного релиза не означает, что для той же версии уже опубликован готовый stable binary/installer. Для установки следует использовать только явно опубликованный и проверяемый дистрибутив соответствующего канала.

## Core

К обязательному ядру Control Center относятся:

- локальная Identity и deny-by-default RBAC;
- обязательная смена первоначального пароля после чистой установки;
- сессии, их инвентаризация и отзыв;
- append-oriented Audit;
- PostgreSQL-backed durable state;
- Desired State / Actual State;
- типизированные Changes и Jobs с проверяемым результатом;
- модель узлов, ролей и их lifecycle;
- Network Management с multi-NIC, зонами, routing, DNS/NTP и firewall policy;
- backup/recovery contracts;
- Monitoring/Health;
- Capacity Planner и advisory-рекомендации по ресурсам и размещению.

Начиная с 0.23.0 пользователь может безопасно просматривать собственные назначения RBAC через read-only self-introspection API. В 0.24.0 добавлена ограниченная process-local защита локального входа от password brute force и credential spraying без раскрытия существования учётной записи.

## Single-node, multi-node и HA

Single-node является самостоятельным поддерживаемым способом использования Control Center. Отказ единственного узла делает management plane недоступным до восстановления, поэтому single-node не следует считать HA.

Multi-node и HA рассматриваются как отдельные проверяемые профили. Наличие соответствующих контрактов в архитектуре не означает автоматически доказанный production failover. Quorum, fencing, replication, switchover и другие HA-возможности считаются поддерживаемыми только для явно опубликованного и проверенного профиля.

## Lifecycle и восстановление

Для узлов и сервисов используются управляемые операции maintenance, drain, replacement и decommission. Stateful workload нельзя переносить как обычный stateless сервис: для него требуется совместимый provider-specific migration/recovery path.

Перед опасными изменениями должны быть известны ожидаемое изменение, риск и blast radius, preflight-проверки, способ проверки результата и rollback/recovery path. Recovery рассматривается как самостоятельная подсистема; backup без проверенного restore не считается достаточным доказательством готовности к восстановлению.

## Сеть

Control Center предусматривает first-class управление сетью, включая multi-NIC, WAN/LAN и другие назначаемые зоны, VLAN/bonding там, где они поддерживаются, routing, DNS/NTP и firewall policy. NAT и port-forwarding не включаются автоматически и требуют явного назначения. Опасные сетевые изменения должны применяться staged-способом с проверкой связности и возможностью отката.

## Capacity Planner

Capacity Planner использует профили нагрузок, фактическую телеметрию, ограничения CPU/RAM/storage/network, тренды и failure reserve. Его задача — отвечать на три практических вопроса: сколько ресурсов безопасно доступно сейчас, когда закончится резерв и что требуется изменить. Текущие Capacity-возможности являются advisory-only и сами по себе не разрешают автоматические инфраструктурные изменения.

## Market

Market содержит устанавливаемые инфраструктурные возможности и не смешивается с Core. Для каждого модуля должны быть определены identity, compatibility/dependencies, permissions, network/storage requirements, capacity profile и lifecycle `Install → Configure → Health → Update → Migrate/Drain → Backup → Restore → Remove`; Failover добавляется только для действительно поддерживаемого provider.

К целевым направлениям Market относятся Directory Services (включая Samba AD и FreeIPA в применимых сценариях), DNS/DHCP, PXE для Windows и Linux, Software Automation для Windows и Linux, Inventory/Compliance, File Services, Monitoring и Backup providers.

## Первый вход

В опубликованной исходной линии после чистой установки создаётся локальный пользователь `admin` с первоначальным паролем `admin`. Первая сессия ограничена до смены пароля: обычная работа разрешается только после задания нового пароля. При обновлении существующий пользовательский пароль не должен сбрасываться обратно к `admin`.
