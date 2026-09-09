# Дорожная карта Control Center

Статус на 09.09.2026: опубликованы исходные релизы `0.4.0`, `0.5.0` и `0.6.0`. Текущий опубликованный baseline — **0.6.0**. Следующая версия `0.7.0` находится на этапе проверки кандидата и **не является опубликованным релизом**.

Версионные номера и этапы ниже описывают последовательность продуктовых возможностей, а не календарные обещания. Возможность считается доступной пользователю только после официальной публикации версии, в которую она фактически вошла.

## 1. Опубликованная основа: 0.4.0 — Distributed Core Contracts

Цель этапа — заложить совместимые данные и API для распределённого управления без необходимости ломать Core при развитии Site, HA, Capacity, Network и Recovery.

В линию входят:

- Node Role model: Management/Controller/Worker/Data/Consensus/Repository/Telemetry/Backup/Edge;
- Site, Management Zone, hierarchical scopes и delegated RBAC boundaries;
- `object_id/scope_id/owner_scope/generation/resource_version`;
- расширенная модель Agent enrollment с roles, site/zone, hardware/network inventory и identity metadata;
- Desired State / Actual State ownership contract;
- Node lifecycle state machine;
- Market Manifest v2 с lifecycle/capacity/recovery/network/dependency metadata;
- Network Interface/Zone data model;
- CapacityObservation/CapacityProfile/Constraint/Recommendation contracts;
- RecoveryPoint/Backup/Restore metadata contracts;
- миграции PostgreSQL и API/OpenAPI для новых сущностей.

## 2. Опубликованная основа: 0.5.0 — Multi-node Operations и Lifecycle

Цель этапа — безопасное управление несколькими узлами из единого management plane.

Ключевые направления:

- runtime управляемого агента и role assignment;
- безопасное enrollment и offline enrollment package;
- package/repository cache;
- placement planning;
- Node Lifecycle: maintenance, drain, replacement, remove/decommission;
- Upgrade Orchestrator с preflight, порядком обновления, update rings и maintenance windows;
- перенос stateless workloads;
- перенос Data role только через replica/switchover abstraction;
- базовая оценка безопасной ёмкости и bottleneck;
- failure-aware поведение: потеря или перезапуск узла не должны создавать ложный Success.

Опасная операция lifecycle должна всегда показывать, что изменится, возможный риск, проверку результата и путь восстановления.

## 3. Текущий опубликованный baseline: 0.6.0 — Site Autonomy и Network Foundation

Цель этапа — автономная разрешённая работа Site при временной потере WAN и безопасная управляемая сеть.

В `0.6.0` опубликованы:

- Site hierarchy без обязательной жёстко заданной Regional-роли;
- локальная Site state model и синхронизация Desired/Actual State;
- делегированные offline-операции в ограниченном scope;
- правила reconciliation после восстановления WAN;
- зоны WAN/LAN/MANAGEMENT/DMZ/CLUSTER/STORAGE/BACKUP;
- запрет межзонной маршрутизации по умолчанию;
- явное назначение Edge Gateway;
- staged network changes с connectivity checks и автоматическим rollback;
- нормализованная network telemetry для оценки ограничений и Capacity Planner.

Наличие WAN+LAN не делает узел маршрутизатором автоматически. Routing, forwarding, NAT и port-forwarding должны включаться только явной поддерживаемой операцией.

## 4. Следующая версия: 0.7.0 — Capacity Intelligence

`0.7.0` пока не опубликован. Текущий кандидат развивает Capacity & Placement Advisor как advisory-only подсистему и не получает права самостоятельно менять инфраструктуру.

В кандидат входят:

- детерминированный forecast нагрузки по нормализованной telemetry;
- текущая нагрузка, темп роста и прогноз на заданный горизонт;
- safety margin;
- confidence оценки качества прогноза;
- what-if сценарии с разной нагрузкой и резервом на отказ узлов;
- повторное использование fail-closed Capacity Assessment;
- сравнение сценариев и выбор безопасного рекомендованного варианта;
- стабильная идентичность результата без исполнительных команд.

Критерий готовности Capacity Planner: он должен достоверно отвечать на три вопроса — **сколько безопасно обслуживаем сейчас, когда закончится резерв и что конкретно рекомендуется изменить**. Forecast и what-if считаются частью продукта только после официального выпуска соответствующей версии.

## 5. Следующий крупный этап — HA Data/Control Plane и Disaster Recovery

Цель — доказанная отказоустойчивость, а не декларация HA.

Архитектурный объём:

- Controller cluster membership и quorum;
- consensus/DCS layer;
- PostgreSQL HA через поддерживаемый provider;
- профили развёртывания для нескольких размеров кластера;
- controlled leader/primary switchover;
- fencing и split-brain protection;
- Backup Repository role;
- PostgreSQL base backup + WAL/PITR;
- Recovery Manager;
- object-level recovery/Recycle Bin/change history;
- high-impact automatic Recovery Point;
- RPO/RTO policies;
- scheduled restore drills;
- восстановление Controller/Site из Desired State;
- degraded safe/read mode при потере quorum.

Критерий готовности: отказ одного или нескольких поддерживаемых узлов не должен приводить к split-brain или недоказанной записи данных; восстановление должно проверяться фактическим restore, а не только наличием backup.

## 6. Следующий крупный этап — Market Platform v2 и инфраструктурные providers

Цель — единый зрелый lifecycle для устанавливаемых возможностей без дублирования Core.

Базовые семейства Market:

- Directory Services с выбором поддерживаемого provider, включая Samba AD и FreeIPA там, где это применимо;
- DNS/DHCP;
- PXE для Windows и Linux;
- Software Automation для Windows и Linux;
- Inventory/Compliance;
- File Services;
- Monitoring;
- Backup;
- дополнительные инфраструктурные сервисы через единый Manifest/lifecycle contract.

Для каждого модуля должны быть определены identity, compatibility, dependencies/conflicts, permissions/capabilities, capacity profile, network/storage requirements, backup/recovery и lifecycle `Install → Configure → Health → Update → Migrate/Drain → Backup → Restore → Remove`. Failover включается в lifecycle только если он действительно поддержан конкретным provider.

Перспективные корпоративные providers включают Mail & Groupware, 1C:Enterprise Server и Secure Web Gateway. Лицензируемые внешние продукты используются только с правомерно предоставленными пользователем дистрибутивами и лицензиями.

## 7. Capacity Planner — текущая кандидатная линия и дальнейшее развитие

После базового `0.7.0` forecast/what-if контура уже подготовлены дополнительные advisory-only кандидаты. Они не меняют опубликованный baseline `0.6.0` и не получают права автоматически изменять инфраструктуру:

- `0.11.0` — Capacity Calibration: сопоставление прогнозируемой и наблюдаемой нагрузки, оценка качества evidence и fail-closed состояние `ready` / `collect-evidence` / `blocked`;
- `0.12.0` — Forecast Correction: консервативная поправка прогноза, привязанная к конкретным forecast/calibration evidence; коэффициент меньше `1` не используется для автоматического снижения плановой нагрузки;
- `0.13.0` — Calibration Trend: оценка устойчивости калибровочного коэффициента во времени и обнаружение drift.

Для кандидатных номеров, для которых отсутствует подтверждённое release-описание, отдельный пользовательский состав в этой дорожной карте не фиксируется. Ни одна из перечисленных аналитических функций не меняет Desired State или Actual State, не выполняет placement/drain/migration/resize и не запускает provider-команды.

Дальнейшее развитие Capacity Planner включает:

- Device/Service Workload Profiles;
- nonlinear capacity curves;
- анализ DB/storage/network bottleneck;
- расширенную self-calibration по фактической telemetry;
- прогноз срока исчерпания резерва;
- what-if по числу устройств, частоте telemetry, Market workloads и изменениям WAN/storage;
- рекомендации по добавлению/переносу ролей и увеличению ресурсов;
- capacity test catalog для официальных модулей;
- confidence score и резерв с учётом отказа узла.

Рекомендация Capacity Planner не является разрешением на изменение инфраструктуры. Любое применение рекомендации проходит обычные Change/Job/RBAC/Audit и recovery boundaries.

## 8. Policy-driven Operations

Автоматическое выполнение допускается только после накопления доказательной базы по lifecycle, capacity, network и recovery.

Перспективные функции:

- policy-driven placement;
- automatic rebalance для явно разрешённых workloads;
- controlled automatic recovery;
- автоматические scale recommendations/actions в заданных пределах;
- mature multi-site/HA/DR certification;
- production capacity baselines.

Автоматическое перемещение stateful workload без provider-specific migration/recovery adapter запрещено.

## 9. Общие требования ко всем следующим возможностям

Для каждой новой функции обязательны:

1. понятный object/data/API/RBAC contract;
2. failure и recovery model;
3. идемпотентное и проверяемое применение;
4. observability и audit;
5. negative/failure/security coverage;
6. upgrade/migration path;
7. пользовательская и эксплуатационная документация;
8. явное разделение между реализованной функцией, предварительным кандидатом и опубликованным релизом.

Core и Market должны оставаться разделёнными. Другие продукты не являются компонентами или обязательными runtime-зависимостями Control Center.
