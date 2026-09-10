# Дорожная карта Control Center

Статус: **опубликованный исходный релиз — 0.17.0; текущий подтверждённый следующий кандидат — 0.18.0**.

Этот документ описывает продуктовую последовательность. Возможность считается доступной пользователю только после официальной публикации версии, в которую она фактически вошла. Архитектурная цель или кандидат не должны описываться как уже опубликованная функция.

## 1. Опубликованная основа

Control Center развивает единый Core с серверной Identity/RBAC, Audit, PostgreSQL-backed state, Changes/Jobs, Desired State / Actual State, Node/Role/Lifecycle, Network Management, Monitoring/Health, Recovery contracts и Capacity Planner. Market остаётся отдельным устанавливаемым слоем поверх общих Core-контрактов.

Single-node является самостоятельным поддерживаемым профилем. Multi-node/HA возможности считаются production-поддерживаемыми только для явно сертифицированных профилей с проверенными quorum/failover/recovery semantics; наличие схем и контрактов само по себе не является доказательством HA.

## 2. Опубликованная Capacity Intelligence линия

Последовательность опубликованных исходных релизов:

- **0.7.0** — deterministic forecast и what-if;
- **0.8.0** — advisory Placement Advice;
- **0.9.0** — Bottleneck Report;
- **0.10.0** — Capacity Horizon;
- **0.11.0** — Capacity Calibration;
- **0.12.0** — conservative Forecast Correction;
- **0.13.0** — Calibration Trend;
- **0.14.0** — benchmark-backed Workload Profile;
- **0.15.0** — bounded nonlinear Workload Curve без extrapolation за пределами evidence;
- **0.16.0** — Workload Curve Efficiency и обнаружение diminishing returns;
- **0.17.0** — Workload Scale Scenario и Scale Options для оценки required workload/headroom и выбора минимального evidence-backed resource factor.

Вся эта линия остаётся **advisory-only**: расчёты не разрешают automatic placement, resize, migration, drain, rebalance, изменение Desired State/Actual State, сетевую перенастройку или provider-команды.

## 3. Текущий кандидат 0.18.0

**0.18.0 — Placement Safety Margin**.

Кандидат формирует детерминированное объясняющее evidence для Placement Advice: workload reserve, bottleneck reserve и ограничивающее измерение, effective safety margin, состояние `blocked` / `boundary` / `headroom` и confidence. Входной Placement Advice должен воспроизводиться из исходного запроса и Node Projections; tampered evidence отклоняется fail-closed.

0.18.0 остаётся кандидатом до отдельной qualification и официальной публикации. Он также не выполняет placement или production mutation.

## 4. Lifecycle и Recovery

Управляемый lifecycle узла охватывает enrollment, active, maintenance, drain, replacement, decommission/remove и recovery после отказа. Перед опасной операцией должны быть определены:

- что изменится и каков blast radius;
- preflight и критерии допуска;
- способ проверки результата;
- rollback/recovery или безопасная компенсация.

Stateful workload нельзя переносить как stateless сервис: требуется provider-specific migration/recovery adapter, достаточный reserve и проверяемое состояние данных.

Recovery развивается как отдельная продуктовая подсистема: Recovery Points, backup metadata/integrity, PostgreSQL backup/PITR там, где профиль это поддерживает, object-level recovery для применимых объектов, измеряемые RPO/RTO и restore drills.

## 5. Managed Network

Network Management является частью Core и должен поддерживать multi-NIC, назначаемые зоны WAN/LAN/MANAGEMENT/DMZ/CLUSTER/STORAGE/BACKUP, routing, VLAN/bonding там, где это поддерживается, DNS/NTP, firewall policy и безопасные staged changes с connectivity verification и автоматическим rollback.

NAT/port-forwarding включаются только явно. Наличие WAN+LAN само по себе не назначает узлу роль Edge Gateway.

## 6. Core и Market

Core содержит Identity/RBAC, Desired/Actual State, Changes/Jobs, Node/Role/Lifecycle, Network, Monitoring/Health, Audit, Recovery contracts, Capacity Planner и общие системные API/security boundaries.

Market содержит устанавливаемые инфраструктурные возможности, включая Directory Services, DNS/DHCP, PXE Windows/Linux, Software Automation Windows/Linux, Inventory/Compliance, File Services, Monitoring/Backup и другие providers через единый module contract.

Для каждого Market-модуля должны быть определены compatibility/dependencies/conflicts, permissions/capabilities, network/storage requirements, capacity profile и lifecycle:

`Install → Configure → Health → Update → Migrate/Drain → Backup → Restore → Remove`

Failover добавляется только когда конкретный provider действительно его поддерживает и это подтверждено тестами.

## 7. Следующие крупные направления

После текущей Capacity-линии продукт продолжает развивать:

- **HA / Disaster Recovery** — controller membership/quorum, consensus/DCS, PostgreSQL HA через поддерживаемый provider, controlled switchover, fencing/split-brain protection, Recovery Manager, PITR/object recovery и restore drills;
- **Market Platform v2** — единый зрелый lifecycle, compatibility/dependencies/conflicts, capacity/network/recovery metadata и сертификация providers;
- **Capacity Intelligence** — дальнейшая evidence-backed оценка вариантов размещения/масштабирования, DB/storage/network bottlenecks и planning confidence;
- **Policy-driven Operations** — только после доказанной failure/recovery/capacity модели и только в явно разрешённых пределах.

Автоматическое перемещение stateful workloads без provider-specific migration/recovery adapter запрещено.

## 8. Аутентификация

В опубликованной исходной линии **0.6.0–0.17.0** чистая установка создаёт локального пользователя `admin` с первоначальным паролем `admin`; при первом входе пароль должен быть изменён, а обычная работа до смены запрещена. При обновлении пользовательский пароль сохраняется.

Отдельный бинарный stable-выпуск **0.3.1** относится к более ранней bootstrap-модели и должен эксплуатироваться по собственной release-документации.

## 9. Критерии готовности

Новая capability готова только когда определены data/API/RBAC semantics, Desired/Actual semantics при изменении состояния, failure/recovery model, validation/idempotency, health/observability/audit, upgrade/migration path и необходимые negative/failure/security tests.

Релиз не считается завершённым, если есть release-blocking defect, release notes расходятся с кодом, install/upgrade/restore path не проверен для заявленной области или документация описывает кандидатные возможности как уже опубликованные.
