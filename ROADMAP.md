# Дорожная карта Control Center

Статус на 10.09.2026: последний опубликованный исходный релиз — **0.14.0**. Отдельный стабильный бинарный канал распространения на момент этой редакции подтверждён до **0.3.1**.

Версионные номера ниже фиксируют фактически опубликованные и подготовленные capability stages. Возможность считается доступной пользователю только после официальной публикации версии, в которую она фактически вошла. Наличие кандидата или архитектурного контракта не означает production-доступность.

## 1. Опубликованная основа

### 0.4.0 — Distributed Core Contracts

Сформированы совместимые распределённые контракты для ролей и scope/site/zone, Desired/Actual State, Node lifecycle, Market Manifest v2, Network, Capacity и Recovery metadata.

### 0.5.0 — Multi-node Operations и Lifecycle foundation

Развиты основы управления несколькими узлами, enrollment/role assignment, maintenance/drain/replacement/decommission, upgrade orchestration и failure-aware поведения.

### 0.6.0 — Site Autonomy и Network Foundation

Опубликованы Site hierarchy и синхронизация Desired/Actual State, делегированная offline-работа в ограниченном scope, сетевые зоны, явная Edge Gateway модель и staged network changes с connectivity checks/rollback.

Наличие WAN+LAN не включает routing, forwarding, NAT или port-forwarding автоматически.

## 2. Опубликованная линия Capacity Intelligence

Capacity Intelligence развивается как **advisory-only** подсистема и не получает права самостоятельно изменять инфраструктуру.

- **0.7.0** — детерминированный workload forecast и what-if;
- **0.8.0** — Placement Advice для уже подходящих узлов;
- **0.9.0** — Bottleneck Report;
- **0.10.0** — Capacity Horizon, включая оценку времени до исчерпания безопасной ёмкости;
- **0.11.0** — Capacity Calibration с fail-closed оценкой качества evidence;
- **0.12.0** — консервативная Forecast Correction;
- **0.13.0** — Calibration Trend для обнаружения дрейфа калибровочной модели во времени;
- **0.14.0** — Benchmark-backed Workload Profile: воспроизводимая безопасная workload boundary по измеренным utilization/error/P95 latency samples.

Общий safety contract линии 0.7–0.14:

- `advisory_only = true`;
- `production_mutation = false`;
- расчёты не меняют Desired State или Actual State;
- не выполняют placement, drain, migration, resize или rebalance;
- не запускают provider-команды;
- не изменяют сеть, storage или workload policy автоматически.

## 3. Текущая кандидатная линия после 0.14.0

Следующие версии существуют как подготовленные кандидаты и не считаются опубликованными возможностями до отдельного официального выпуска:

- **0.15.0** — bounded nonlinear Workload Curve: piecewise interpolation только внутри измеренного диапазона и запрет extrapolation;
- **0.16.0** — Workload Curve Efficiency: оценка эффективности добавления ресурсов и обнаружение diminishing returns;
- **0.17.0** — Workload Scale Scenario: оценка требуемой нагрузки и headroom на конкретном resource factor.

Все эти кандидаты сохраняют advisory-only и non-mutation границы. Их наличие не разрешает automatic placement, scaling, migration или закупку ресурсов.

## 4. Single-node, multi-node и HA

Single-node является самостоятельным способом использования Control Center.

Целевая multi-node/HA архитектура предусматривает распределение ролей, maintenance/drain, replacement/decommission, quorum/fencing, controlled switchover, восстановление после потери узлов и HA данных. Однако конкретный HA-профиль считается поддержанным только после фактической реализации, failure/recovery qualification и публикации соответствующей версии.

Наличие multi-node контрактов не является доказательством production failover.

## 5. Lifecycle, Upgrade и Recovery

Node lifecycle развивается вокруг состояний enrollment, active, maintenance, drain, replacement и decommission/remove. Опасная операция должна иметь:

1. понятный diff/план изменения;
2. оценку риска и blast radius;
3. preflight;
4. проверку результата по Actual State/Health;
5. recovery/rollback или безопасную compensation модель;
6. Audit/evidence.

Stateful workload нельзя переносить как обычный stateless сервис без provider-specific migration/recovery adapter.

Recovery является отдельной продуктовой подсистемой. Целевой объём включает Recovery Points, backup metadata, restore verification, PostgreSQL base backup + WAL/PITR для поддерживаемого профиля, object-level recovery, RPO/RTO и регулярные restore drills.

## 6. Managed Network

Network Management является частью Core. Целевая модель включает multi-NIC, назначаемые зоны, VLAN/bonding там, где это поддерживается, routing, DNS/NTP, firewall policy, NAT/port-forwarding только при явном включении, staged changes, connectivity verification и automatic rollback при потере управляемости.

Сетевое изменение не должно оставлять узел недоступным без заранее определённого recovery path.

## 7. Core и Market

**Core** содержит обязательные платформенные границы: Identity/RBAC, Desired/Actual State, Changes/Jobs, Node/Role/Lifecycle, Network, Monitoring/Health, Audit, Backup/Recovery contracts, Capacity Planner foundation и общие security/API contracts.

**Market** содержит устанавливаемые инфраструктурные возможности. Для модуля должны быть определены identity, compatibility/dependencies/conflicts, permissions/capabilities, network/storage requirements, capacity profile и lifecycle.

Зафиксированные направления Market включают Directory Services, DNS/DHCP, PXE Windows/Linux, Software Automation Windows/Linux, Inventory/Compliance, File Services, Monitoring, Backup и другие поддерживаемые providers.

Наличие направления в roadmap не означает его присутствие в текущем опубликованном релизе.

## 8. Следующие крупные capability stages

Номера этих этапов не закрепляются заранее, чтобы не переиспользовать уже занятые версии.

### HA / Disaster Recovery

- Controller membership и quorum;
- consensus/DCS layer;
- PostgreSQL HA через поддерживаемый provider;
- controlled switchover;
- fencing/split-brain protection;
- Backup Repository role;
- PITR и Recovery Manager;
- object recovery;
- RPO/RTO policies и restore drills.

### Market Platform v2

- зрелый module lifecycle;
- compatibility/dependency/conflict model;
- capacity/network/recovery metadata;
- сертификация supported providers;
- безопасные install/update/migrate/backup/restore/remove procedures.

### Policy-driven Operations

Автоматизация инфраструктурных изменений допускается только после доказанной lifecycle/capacity/network/recovery модели и только в явно разрешённых пределах:

- policy-driven placement;
- controlled rebalance;
- controlled automatic recovery;
- scale recommendations/actions через обычные RBAC/Change/Job/Audit/recovery boundaries.

Автоматическое перемещение stateful workload без provider-specific migration/recovery adapter запрещено.

## 9. Критерии готовности новой возможности

Для каждой новой capability обязательны:

1. object/data/API/RBAC contract;
2. Desired/Actual semantics, если функция меняет состояние;
3. failure/recovery model;
4. validation, idempotency и stale-state protection;
5. health/observability/audit semantics;
6. negative/failure/security coverage по уровню риска;
7. upgrade/migration path;
8. backup/restore semantics для stateful data;
9. актуальная пользовательская и эксплуатационная документация;
10. соответствие заявленного поведения фактической реализации.

## 10. Граница выпуска

Кандидат не становится опубликованной функцией только из-за наличия ветки, версии или release notes. Официальный выпуск требует завершённого acceptance для exact revision и отсутствия release-blocking security/recovery дефектов.

Отдельный binary stable channel имеет собственную release identity. Исходный релиз более новой версии не должен описываться как уже доступный stable binary, пока соответствующий бинарный выпуск фактически не опубликован.