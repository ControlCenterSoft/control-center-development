# Дорожная карта Control Center

Статус на 10.09.2026: опубликованы исходные релизы `0.4.0`, `0.5.0` и `0.6.0`. Последний опубликованный исходный релиз — **0.6.0**. Следующие версии `0.7.0`–`0.17.0` относятся к предварительной Capacity Intelligence линии и не являются опубликованными релизами.

Версионные номера ниже отражают фактический состав уже сформированных релизных пакетов. Архитектурные направления без подтверждённого release scope не получают преждевременный номер версии.

## 1. Опубликованная основа: 0.4.0 — Distributed Core Contracts

Этап заложил совместимые данные и API для распределённого управления без необходимости ломать Core при развитии Site, HA, Capacity, Network и Recovery.

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

Этап развивает безопасное управление несколькими узлами из единого management plane.

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

Опасная lifecycle-операция должна показывать, что изменится, возможный риск, проверку результата и путь восстановления.

## 3. Последний опубликованный исходный релиз: 0.6.0 — Site Autonomy и Network Foundation

`0.6.0` включает:

- Site hierarchy без обязательной жёстко заданной Regional-роли;
- локальную Site state model и синхронизацию Desired/Actual State;
- делегированные offline-операции в ограниченном scope;
- правила reconciliation после восстановления WAN;
- зоны WAN/LAN/MANAGEMENT/DMZ/CLUSTER/STORAGE/BACKUP;
- запрет межзонной маршрутизации по умолчанию;
- явное назначение Edge Gateway;
- staged network change contracts с connectivity checks и rollback semantics;
- нормализованную network telemetry для оценки ограничений и Capacity Planner.

Наличие WAN+LAN не делает узел маршрутизатором автоматически. Routing, forwarding, NAT и port-forwarding включаются только явной поддерживаемой операцией. Опубликованный `0.6.0` не следует трактовать как доказательство уже доступной production network mutation для всех описанных целевых сценариев.

## 4. Кандидатная Capacity Intelligence линия

Все версии этого раздела являются предварительными до отдельной официальной публикации и сохраняют `advisory_only` / `production_mutation=false` границу там, где она зафиксирована соответствующим контрактом.

### 0.7.0 — Forecast и What-if

- детерминированный forecast нагрузки по нормализованной telemetry;
- текущая нагрузка, темп роста и прогноз на заданный горизонт;
- safety margin и confidence;
- what-if сценарии с разной нагрузкой и failure reserve;
- повторное использование fail-closed Capacity Assessment.

### 0.8.0 — Placement Advice

- детерминированный рекомендательный выбор уже подходящего узла;
- safe capacity, bottleneck reserve, confidence и общий failure reserve;
- отсутствие автоматического назначения роли или изменения Desired State.

### 0.9.0 — Bottleneck Report

- read-only оценка узких мест по безопасной ёмкости и ограничивающей метрике;
- состояния `critical`, `warning`, `unknown`, `healthy`;
- fail-closed рекомендации `add-role-capacity`, `collect-evidence`, `adjust-workload-policy` или `none`.

### 0.10.0 — Capacity Horizon

- срок до достижения безопасной границы при наблюдаемом тренде;
- risk classification `healthy` / `warning` / `critical` / `unknown`;
- расчёт относительно safe capacity после failure reserve.

### 0.11.0 — Capacity Calibration

- сопоставление прогнозируемой и наблюдаемой нагрузки;
- оценка ошибки и качества evidence;
- fail-closed состояния `ready`, `collect-evidence`, `blocked`;
- bounded calibration multiplier без автоматической мутации Capacity Profile.

### 0.12.0 — Forecast Correction

- консервативная поправка конкретного forecast по конкретной calibration evidence;
- коэффициент меньше `1` не используется для снижения planning workload;
- mismatch scope/workload/evidence блокируется.

### 0.13.0 — Calibration Trend

- оценка устойчивости calibration multiplier во времени;
- обнаружение drift;
- состояния `stable`, `drifting`, `collect-evidence`, `blocked`.

### 0.14.0 — Benchmark-backed Workload Profile

- формирование воспроизводимой workload boundary из измеренных utilization/error/P95 latency samples;
- безопасная нагрузка после обязательного reserve;
- обнаружение saturation boundary;
- блокировка немонотонного или недостаточного evidence.

### 0.15.0 — Nonlinear Workload Curve

- bounded nonlinear curve по нескольким benchmark-backed workload profiles;
- piecewise interpolation внутри измеренного диапазона;
- запрет extrapolation за пределы наблюдаемого диапазона;
- консервативный confidence.

### 0.16.0 — Workload Curve Efficiency

- анализ эффективности масштабирования по сегментам измеренной кривой;
- обнаружение diminishing returns;
- рекомендация исследовать bottleneck вместо автоматического увеличения ресурсов.

### 0.17.0 — Workload Scale Scenario

- оценка требуемой workload и headroom на конкретном resource factor;
- использование только измеренного диапазона Workload Curve;
- `insufficient-capacity`, `diminishing-returns`, `collect-evidence` и fail-closed evidence handling;
- отсутствие автоматического resize/placement/migration/rebalance.

Критерий зрелости Capacity Planner: он должен достоверно отвечать на три вопроса — **сколько безопасно обслуживаем сейчас, когда закончится резерв и что конкретно рекомендуется изменить**. Рекомендация сама по себе не является разрешением на изменение инфраструктуры.

## 5. HA Data/Control Plane и Disaster Recovery

Следующий крупный архитектурный этап после подтверждения необходимых базовых контрактов включает:

- Controller cluster membership и quorum;
- consensus/DCS layer;
- PostgreSQL HA через поддерживаемый provider;
- controlled leader/primary switchover;
- fencing и split-brain protection;
- Backup Repository role;
- PostgreSQL base backup + WAL/PITR;
- Recovery Manager;
- object-level recovery/Recycle Bin/change history;
- high-impact Recovery Point;
- RPO/RTO policies и restore drills;
- восстановление Controller/Site из Desired State;
- degraded safe/read mode при потере quorum.

HA считается поддержанной только в пределах опубликованного и проверенного failure/recovery profile. Наличие целевого контракта не является доказательством production failover.

## 6. Market Platform v2 и инфраструктурные providers

Цель — единый зрелый lifecycle устанавливаемых возможностей без дублирования Core.

Базовые семейства Market:

- Directory Services с поддерживаемыми provider, включая Samba AD и FreeIPA там, где это применимо;
- DNS/DHCP;
- PXE для Windows и Linux;
- Software Automation для Windows и Linux;
- Inventory/Compliance;
- File Services;
- Monitoring;
- Backup;
- дополнительные инфраструктурные сервисы через единый Manifest/lifecycle contract.

Для каждого модуля должны быть определены identity, compatibility, dependencies/conflicts, permissions/capabilities, capacity profile, network/storage requirements, backup/recovery и lifecycle `Install → Configure → Health → Update → Migrate/Drain → Backup → Restore → Remove`. Failover включается только если он действительно поддержан конкретным provider.

Перспективные корпоративные providers могут включать Mail & Groupware, 1C:Enterprise Server и Secure Web Gateway. Лицензируемые внешние продукты используются только с правомерно предоставленными пользователем дистрибутивами и лицензиями.

## 7. Policy-driven Operations

Автоматическое выполнение допускается только после накопления доказательной базы по lifecycle, capacity, network и recovery.

Перспективные функции:

- policy-driven placement;
- controlled automatic rebalance для явно разрешённых workloads;
- automatic recovery только в заранее заданных пределах;
- scale recommendations/actions после доказанной capacity/failure модели;
- mature multi-site/HA/DR certification.

Автоматическое перемещение stateful workload без provider-specific migration/recovery adapter запрещено.

## 8. Общие требования ко всем следующим возможностям

Для каждой новой функции обязательны:

1. понятный object/data/API/RBAC contract;
2. failure и recovery model;
3. идемпотентное и проверяемое применение;
4. observability и audit;
5. negative/failure/security coverage по уровню риска;
6. upgrade/migration path;
7. пользовательская и эксплуатационная документация;
8. явное разделение между реализованной функцией, предварительным кандидатом и опубликованным релизом.

Core и Market должны оставаться разделёнными. Другие продукты не являются компонентами или обязательными runtime-зависимостями Control Center.
