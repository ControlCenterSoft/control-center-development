# Дорожная карта Control Center

Статус на 10.09.2026: последний опубликованный исходный релиз — **0.10.0**. Отдельный стабильный бинарный канал опубликован до **0.3.1**. Версии выше `0.10.0` считаются предварительными до отдельной официальной публикации.

Версионные номера ниже описывают последовательность продуктовых возможностей, а не календарные обещания. Возможность считается доступной пользователю только после официального выпуска версии, в которую она фактически вошла.

## Опубликованная основа

### 0.4.0 — Distributed Core Contracts

Сформированы совместимые распределённые контракты ролей и scopes, Desired/Actual State, Node lifecycle, Market Manifest v2, network/capacity/recovery metadata и versioned object semantics.

### 0.5.0 — Multi-node Operations и Lifecycle

Развиты agent/enrollment foundations, управление несколькими узлами, lifecycle-планирование, placement/capacity foundations и безопасная модель длительных операций.

### 0.6.0 — Site Autonomy и Network Foundation

Добавлены Site hierarchy и автономная работа в делегированном scope, синхронизация Desired/Actual State, network zones, явная роль Edge Gateway, staged network changes с connectivity checks/rollback и network telemetry для Capacity Planner.

Наличие WAN+LAN не делает узел маршрутизатором автоматически. Routing, forwarding, NAT и port-forwarding включаются только отдельной явно разрешённой операцией.

### 0.7.0 — Capacity Intelligence: Forecast и What-if

Опубликованы детерминированный прогноз нагрузки и what-if моделирование с failure reserve. Контур остаётся advisory-only.

### 0.8.0 — Placement Advice

Опубликован детерминированный рекомендательный выбор уже подходящего узла с учётом safe capacity, bottleneck reserve, confidence и общего failure reserve. Алгоритм не назначает роли и не меняет Desired State.

### 0.9.0 — Bottleneck Report

Опубликован детерминированный отчёт по узким местам парка. Он сопоставляет safe-capacity reserve и фактически ограничивающую метрику, классифицирует состояние и формирует только рекомендательное действие.

### 0.10.0 — Capacity Horizon

Опубликован детерминированный прогноз времени до исчерпания безопасной ёмкости на основе workload forecast и failure-aware fleet assessment. Capacity Horizon учитывает текущий безопасный резерв, темп роста и явно заданные warning/critical окна, оставаясь advisory-only. Версия `0.10.0` не выполняет placement, resize, drain, migration или автоматическое изменение инфраструктуры.

## Кандидатная Capacity-линия после 0.10.0

Следующие версии существуют как предварительные кандидаты и не являются опубликованными возможностями:

- `0.11.0` — Capacity Calibration: сопоставление прогнозируемой и наблюдаемой нагрузки с fail-closed оценкой evidence;
- `0.12.0` — Forecast Correction: консервативная поправка прогноза без автоматического снижения planning workload;
- `0.13.0` — Calibration Trend: обнаружение дрейфа калибровочного коэффициента во времени;
- `0.14.0` — benchmark-backed Workload Profile;
- `0.15.0` — bounded nonlinear Workload Curve без extrapolation за измеренный диапазон;
- `0.16.0` — Workload Curve Efficiency с обнаружением diminishing returns;
- `0.17.0` — Workload Scale Scenario для проверки требуемой нагрузки и headroom на заданном resource factor.

Все эти Capacity-возможности остаются **advisory-only**: они не меняют Desired State или Actual State, не выполняют placement/drain/migration/resize, не запускают provider-команды и не являются разрешением на автоматическое масштабирование.

## Следующие крупные продуктовые этапы

### HA / Disaster Recovery

Целевая область включает Controller membership и quorum, consensus/DCS layer, PostgreSQL HA через поддерживаемый provider, controlled switchover, fencing/split-brain protection, Backup Repository, PITR, Recovery Manager, object-level recovery, RPO/RTO и restore drills.

HA считается поддержанным только после фактической реализации и проверки соответствующего failure/recovery-профиля. Наличие архитектурного контракта не является доказательством production failover.

### Market Platform

Цель — единый зрелый lifecycle устанавливаемых возможностей. Основные направления Market: Directory Services, DNS/DHCP, PXE Windows/Linux, Software Automation Windows/Linux, Inventory/Compliance, File Services, Monitoring, Backup и другие infrastructure providers через единый module contract.

Для каждого модуля должны быть определены identity, compatibility, dependencies/conflicts, permissions/capabilities, capacity profile, network/storage requirements, backup/recovery и lifecycle `Install → Configure → Health → Update → Migrate/Drain → Backup → Restore → Remove`.

### Policy-driven Operations

Автоматическое применение рекомендаций допускается только после доказанной capacity/failure/recovery модели и только в явно разрешённых пределах. Stateful workload нельзя автоматически переносить без provider-specific migration/recovery adapter.

## Общие продуктовые инварианты

Для каждой новой возможности обязательны:

1. object/data/API/RBAC contract;
2. Desired/Actual semantics для изменяющих состояние функций;
3. failure и recovery model;
4. idempotency и защита от stale state;
5. observability и Audit;
6. negative/failure/security coverage по уровню риска;
7. upgrade/migration path;
8. backup/restore semantics для stateful данных;
9. явное разделение между реализованным кандидатом и опубликованной функцией.

Core и Market остаются разделёнными. Control Center является самостоятельным продуктом и не имеет обязательной runtime-зависимости от других продуктов.
