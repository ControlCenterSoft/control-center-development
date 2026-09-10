# Дорожная карта Control Center

Статус на 10.09.2026: опубликованы исходные релизы `0.4.0`–`0.9.0`. Текущий опубликованный исходный релиз — **0.9.0**. Отдельный стабильный бинарный канал распространения пока подтверждён до `0.3.1`.

Версионные номера ниже отражают фактический опубликованный или подготовленный состав возможностей, а не календарные обещания. Возможность считается доступной пользователю только после официальной публикации версии, в которую она фактически вошла.

## 1. Опубликованная основа 0.4–0.6

### 0.4.0 — Distributed Core Contracts

Сформированы совместимые контракты для ролей и областей управления, Site/Management Zone, Desired/Actual State, lifecycle узлов, Market Manifest v2, Network, Capacity и Recovery metadata.

### 0.5.0 — Multi-node Operations и Lifecycle

Развиты управление несколькими узлами, enrollment, lifecycle, upgrade orchestration, безопасное размещение и базовая capacity-модель. Stateful workload не должен переноситься как обычный stateless сервис без provider-specific migration/recovery механизма.

### 0.6.0 — Site Autonomy и Network Foundation

Опубликованы Site hierarchy, синхронизация Desired/Actual State, делегированные offline-операции в ограниченном scope, сетевые зоны WAN/LAN/MANAGEMENT/DMZ/CLUSTER/STORAGE/BACKUP, явная роль Edge Gateway, staged network changes с connectivity checks/rollback и нормализованная network telemetry.

Наличие WAN+LAN не включает routing, forwarding, NAT или port-forwarding автоматически. Такие возможности требуют отдельной явно разрешённой операции.

## 2. Опубликованная Capacity Intelligence линия 0.7–0.9

### 0.7.0 — Forecast и What-if

Добавлены детерминированный forecast нагрузки и what-if сценарии с учётом safe capacity, confidence и failure reserve. Результаты являются advisory-only.

### 0.8.0 — Placement Advice

Добавлен детерминированный рекомендательный выбор уже подходящего узла для дополнительной нагрузки. Алгоритм не назначает роли и не меняет Desired State.

### 0.9.0 — Bottleneck Report

Добавлен детерминированный отчёт по узким местам, который сопоставляет safe-capacity reserve и фактически ограничивающую метрику, учитывает общий failure reserve парка и выдаёт только рекомендательное действие.

Вся линия 0.7–0.9 остаётся **advisory-only**: она не выполняет placement, migration, resize, сетевые изменения или закупку ресурсов автоматически.

## 3. Подготовленная кандидатная линия после 0.9.0

Следующие версии остаются предварительными до отдельной официальной публикации:

- `0.10.0` — Capacity Horizon;
- `0.11.0` — Capacity Calibration;
- `0.12.0` — Forecast Correction;
- `0.13.0` — Calibration Trend;
- `0.14.0` — benchmark-backed Workload Profile;
- `0.15.0` — bounded nonlinear Workload Curve;
- `0.16.0` — Workload Curve Efficiency;
- `0.17.0` — Workload Scale Scenario.

Эти аналитические этапы не получают права автоматически менять Desired State или Actual State, выполнять placement/drain/migration/resize либо запускать provider-команды.

## 4. Single-node, multi-node и HA

Single-node является самостоятельным способом использования Control Center. Целевая multi-node/HA архитектура предусматривает распределение ролей, maintenance/drain, replacement/decommission, восстановление после потери узлов, quorum/fencing/split-brain protection и HA для Control Plane/данных там, где это поддерживает конкретный профиль.

Наличие архитектурного HA-контракта не является доказательством production failover. Возможность считается поддержанной только после фактической сертификации failure/recovery соответствующей версии и роли.

## 5. Lifecycle и Recovery

Жизненный цикл узла включает enrollment, active, maintenance, drain, replacement и decommission/remove. Перед опасной операцией должны быть известны предполагаемые изменения, риск и blast radius, предварительные проверки, способ проверки результата и путь rollback/recovery.

Recovery рассматривается как отдельная продуктовая подсистема. Целевая модель включает Recovery Points, backup metadata и integrity verification, restore, PostgreSQL base backup + WAL/PITR для поддерживаемых профилей, object-level recovery, измеряемые RPO/RTO и restore drills.

Backup без проверенного restore не считается доказанной готовностью к восстановлению.

## 6. Managed Network

Network Management является частью Core. Целевая модель поддерживает multi-NIC, WAN/LAN и другие зоны, VLAN/bonding там, где они поддерживаются, routing, DNS/NTP, firewall policy и NAT/port-forwarding только при явном включении.

Рискованные сетевые изменения выполняются staged: validation → временное применение → connectivity verification → confirm. При потере управляемости должен существовать автоматический rollback/recovery path.

## 7. Core и Market

**Core** включает Identity/RBAC, Desired/Actual State, Changes/Jobs, Node/Role/Lifecycle, Network, Monitoring/Health, Audit, Backup/Recovery contracts, Capacity Planner foundation и системные API/security boundaries.

**Market** содержит устанавливаемые инфраструктурные возможности. Для каждого модуля определяются identity, compatibility/dependencies/conflicts, permissions/capabilities, network/storage requirements, capacity profile и lifecycle.

К направлениям Market относятся Directory Services с поддерживаемыми providers, включая Samba AD и FreeIPA там, где применимо; DNS/DHCP; PXE Windows/Linux; Software Automation Windows/Linux; IT Asset Inventory; Software Inventory & Compliance; File Services; Monitoring; Backup и другие совместимые infrastructure providers.

Базовый lifecycle модуля: `Install → Configure → Health → Update → Migrate/Drain → Backup → Restore → Remove`. Failover добавляется только когда конкретный provider действительно его поддерживает.

## 8. Capacity Planner — дальнейшее развитие

Целевая Capacity & Placement подсистема использует hardware inventory, фактическую telemetry, workload profiles, Market Capacity Profiles, DB/network/storage characteristics, историю роста и результаты измерений.

Она должна отвечать на три практических вопроса: **сколько безопасно обслуживаем сейчас, когда закончится резерв и что конкретно рекомендуется изменить**. Рекомендация сама по себе не является разрешением на инфраструктурное изменение.

## 9. Policy-driven Operations

Автоматическое выполнение допускается только в заранее определённых безопасных пределах после доказанной модели lifecycle, capacity, networking и recovery. Перспективные функции включают policy-driven placement, controlled automatic rebalance/recovery и scale actions для явно разрешённых workloads.

Автоматическое перемещение stateful workload без provider-specific migration/recovery adapter запрещено.

## 10. Общие критерии новых возможностей

Для новой capability должны быть определены object/data/API contract, RBAC permissions, Desired/Actual semantics, failure/recovery model, validation/idempotency, observability/audit, negative/failure/security coverage, upgrade/migration path и документация. Для stateful data обязательна backup/restore model.

Control Center остаётся самостоятельным продуктом. В продуктовой документации не должны присутствовать сведения о внутренней инфраструктуре разработки, служебных адресах, секретах, ключах или непользовательской operational information.
