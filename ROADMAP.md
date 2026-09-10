# Дорожная карта Control Center

Статус на 10.09.2026: последний опубликованный исходный релиз — **0.13.0**. Отдельный стабильный бинарный канал распространения подтверждён до **0.3.1**. Наличие кандидатной версии не означает пользовательскую доступность до её официальной публикации.

## 1. Опубликованная основа 0.4–0.6

### 0.4.0 — Distributed Core Contracts

Сформированы совместимые контракты распределённых объектов, ролей, Site/Zone, Desired/Actual State, lifecycle узлов, Market Manifest v2, Network, Capacity и Recovery metadata.

### 0.5.0 — Multi-node Operations и Lifecycle

Развиты Agent/enrollment foundations, role assignment, lifecycle узлов, placement planning, update orchestration и failure-aware модель выполнения операций.

### 0.6.0 — Site Autonomy и Network Foundation

Опубликованы Site hierarchy, делегированная автономная работа в ограниченном scope, синхронизация Desired/Actual State, network zones, явный Edge Gateway, staged network changes с connectivity verification/rollback и нормализованная network telemetry.

Наличие WAN+LAN не включает routing, forwarding, NAT или port-forwarding автоматически.

## 2. Опубликованная Capacity Intelligence линия 0.7–0.13

Capacity-возможности этой линии являются **advisory-only** и сами по себе не разрешают изменение инфраструктуры.

- **0.7.0** — детерминированный workload forecast и what-if planning с failure reserve;
- **0.8.0** — Placement Advice для выбора подходящего уже существующего узла без автоматического назначения роли;
- **0.9.0** — Bottleneck Report с анализом safe-capacity и ограничивающих метрик;
- **0.10.0** — Capacity Horizon: оценка времени до исчерпания безопасной ёмкости;
- **0.11.0** — Capacity Calibration: сопоставление прогнозируемой и наблюдаемой нагрузки с fail-closed оценкой качества evidence;
- **0.12.0** — Forecast Correction: консервативная поправка прогноза по конкретному calibration evidence; коэффициент меньше `1` не используется для снижения planning workload;
- **0.13.0** — Calibration Trend: анализ устойчивости калибровки во времени и обнаружение drift.

Критерий зрелости Capacity Planner: система должна достоверно отвечать, сколько нагрузки безопасно обслуживается сейчас, когда закончится резерв и какое изменение рекомендуется. Рекомендация не является разрешением на mutation: её применение проходит обычные RBAC/Change/Job/Audit/recovery boundaries.

## 3. Текущая кандидатная Capacity-линия после 0.13.0

Следующие версии существуют как предварительные capability-кандидаты и не считаются опубликованными возможностями до отдельного выпуска:

- **0.14.0 — Benchmark-backed Workload Profile**: воспроизводимый профиль по измеренным utilization/error/P95 latency samples с safety reserve;
- **0.15.0 — Workload Curve**: bounded nonlinear capacity curve с интерполяцией только внутри измеренного диапазона и без недоказанной extrapolation;
- **0.16.0 — Workload Curve Efficiency**: анализ изменения эффективности и обнаружение diminishing returns;
- **0.17.0 — Workload Scale Scenario**: оценка требуемой нагрузки и headroom для заданного resource factor.

Все перечисленные кандидаты сохраняют `advisory_only=true` и не выполняют placement, drain, migration, resize, сетевые изменения или provider-команды автоматически.

## 4. HA / Disaster Recovery

Цель — доказанная отказоустойчивость, а не декларация HA.

Целевая архитектура включает:

- Controller membership и quorum;
- consensus/DCS layer;
- PostgreSQL HA через поддерживаемый provider;
- controlled switchover/failover;
- fencing и split-brain protection;
- Backup Repository role;
- PostgreSQL base backup + WAL/PITR;
- Recovery Manager и object-level recovery;
- Recovery Points перед высокорисковыми изменениями;
- измеряемые RPO/RTO и регулярные restore drills;
- восстановление Controller/Site из Desired State и сохранённых данных;
- безопасный ограниченный режим при потере quorum.

HA считается поддерживаемым только для явно опубликованного и проверенного профиля. Несколько узлов без подтверждённых quorum/fencing/failover/recovery гарантий не являются production HA.

## 5. Market Platform v2

Цель — единый зрелый lifecycle устанавливаемых инфраструктурных возможностей.

Базовые семейства Market:

- Directory Services с поддерживаемыми Samba AD и FreeIPA providers;
- DNS/DHCP;
- PXE для Windows и Linux;
- Software Automation для Windows и Linux;
- IT Asset Inventory и Software Inventory/Compliance;
- File Services;
- Monitoring;
- Backup;
- дополнительные инфраструктурные providers через единый module contract.

Для каждого модуля должны быть определены identity, compatibility/dependencies/conflicts, permissions/capabilities, network/storage requirements, Capacity Profile и lifecycle:

`Install → Configure → Health → Update → Migrate/Drain → Backup → Restore → Remove`

Failover добавляется только когда конкретный provider действительно его поддерживает и прошёл соответствующую проверку.

## 6. Node Lifecycle и Upgrade

Node Lifecycle остаётся частью Core и предусматривает enrollment, active, maintenance, drain, replacement и decommission/remove.

Перед опасной операцией система должна показывать:

- что изменится;
- риск и blast radius;
- preflight;
- способ проверки результата;
- rollback/recovery или безопасную компенсацию.

Stateful workload нельзя переносить как stateless сервис без provider-specific migration/recovery adapter и подтверждённого состояния данных.

Обновление multi-node/HA выполняется последовательно с проверкой совместимости, quorum, capacity reserve, backup/recovery point и состояния зависимостей.

## 7. Managed Network

Network Management является частью Core и развивается вокруг следующих инвариантов:

- multi-NIC и назначаемые зоны;
- VLAN/bonding там, где это поддерживается;
- routing, DNS/NTP и firewall policy;
- NAT/port-forwarding только при явном включении;
- staged changes с connectivity verification;
- automatic rollback при потере управляемости.

Сетевое изменение не должно оставлять узел недоступным без заранее определённого recovery path.

## 8. Policy-driven Operations

Автоматическое выполнение допустимо только после доказанной зрелости lifecycle, capacity, network и recovery контуров.

Перспективные направления:

- policy-driven placement;
- controlled automatic rebalance для явно разрешённых workloads;
- automatic recovery только в заранее заданных пределах;
- scale recommendations/actions после доказанной capacity/failure модели;
- mature multi-site/HA/DR certification.

Автоматическое перемещение stateful workload без provider-specific migration/recovery adapter запрещено.

## 9. Общие критерии готовности capability

Новая возможность считается готовой только если определены и проверены object/data/API contract, RBAC permissions, Desired/Actual semantics, failure/recovery model, validation/idempotency, health/observability/audit semantics, negative/failure/security coverage, upgrade/migration path и документация. Для stateful data обязательна backup/restore model.

Функция считается доступной пользователю только после официальной публикации версии, в которую она фактически вошла.
