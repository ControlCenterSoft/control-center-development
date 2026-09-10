# Control Center

Control Center — самостоятельная платформа централизованного управления серверной и пользовательской ИТ-инфраструктурой с типизированной, проверяемой и аудируемой моделью выполнения операций.

**Последний опубликованный исходный релиз:** `0.16.0`.

Отдельный стабильный бинарный канал распространения на момент этой редакции подтверждён до `0.3.1`. Более новый исходный релиз не означает, что бинарный пакет той же версии уже опубликован в отдельном stable-канале.

## Назначение

Control Center предоставляет единый контур управления узлами, состоянием, изменениями, заданиями, инфраструктурными сервисами и устанавливаемыми модулями. Платформа строится вокруг разделения Desired State и Actual State: администратор задаёт требуемое состояние, система планирует изменение, проверяет ограничения и только после необходимых допусков может применять поддерживаемую операцию.

## Опубликованная функциональная линия

Линия до `0.16.0` включает базовые Core-контракты, multi-node lifecycle foundation, Site Autonomy и Network Foundation, а также последовательное развитие advisory Capacity Intelligence.

Опубликованные Capacity-возможности:

- `0.7.0` — детерминированный workload forecast и what-if;
- `0.8.0` — advisory Placement Advice;
- `0.9.0` — Bottleneck Report;
- `0.10.0` — Capacity Horizon;
- `0.11.0` — Capacity Calibration;
- `0.12.0` — консервативная Forecast Correction;
- `0.13.0` — Calibration Trend;
- `0.14.0` — benchmark-backed Workload Profile;
- `0.15.0` — bounded nonlinear Workload Curve с piecewise interpolation только внутри измеренного диапазона;
- `0.16.0` — Workload Curve Efficiency с обнаружением diminishing returns и fail-closed проверкой malformed/non-increasing evidence.

Эта линия остаётся аналитической: она не разрешает automatic placement, migration, resize, rebalance или иные production mutations.

## Core

К обязательным границам Core относятся Identity и deny-by-default RBAC, Desired/Actual State, Changes и durable Jobs, типизированные действия вместо generic shell API, Nodes/Roles/Lifecycle, Network, Monitoring/Health, Audit, Backup/Recovery contracts, Capacity Planner foundation и общие API/security boundaries.

## Single-node и multi-node/HA

Single-node является самостоятельным способом использования Control Center и не требует второго сервера. Архитектура предусматривает multi-node, распределение ролей, maintenance/drain, replacement/decommission, восстановление и HA-профили. Однако HA считается поддержанным только для тех ролей и версий, для которых опубликованы и проверены quorum/fencing/failover/recovery процедуры. Наличие архитектурного контракта само по себе не является доказательством production failover.

## Сеть

Network Management является частью Core. Модель предусматривает multi-NIC, WAN/LAN и другие зоны, VLAN/bonding там, где они поддерживаются, routing, DNS/NTP, firewall policy и staged network changes с проверкой связности и rollback. Наличие WAN+LAN **не включает routing, forwarding, NAT или port-forwarding автоматически**.

## Capacity Planner

Capacity Planner оценивает безопасную ёмкость, bottleneck, запас ресурсов, прогноз исчерпания резерва, what-if сценарии и рекомендации по изменению размещения или ресурсов. Опубликованная линия 0.7–0.16 остаётся advisory-only: расчётная рекомендация не является разрешением на изменение инфраструктуры и сохраняет обычные RBAC/Change/Job/Audit/recovery boundaries.

## Core и Market

Core содержит обязательные платформенные функции. Market содержит устанавливаемые инфраструктурные возможности с отдельными identity, compatibility/dependency metadata, permissions/capabilities, storage/network requirements, capacity profile и lifecycle. К направлениям Market относятся Directory Services, DNS/DHCP, PXE Windows/Linux, Software Automation Windows/Linux, Inventory/Compliance, File Services, Monitoring, Backup и другие поддерживаемые infrastructure providers. Наличие направления в roadmap не означает его присутствие в текущем опубликованном релизе.

## Первый вход

Для опубликованной исходной линии начиная с `0.6.0` подтверждена локальная bootstrap-политика: после чистой установки создаётся пользователь `admin` с первоначальным паролем `admin`. При первом входе пароль необходимо сменить; до смены обычная работа с системой запрещена. При обновлении существующий пользовательский пароль не сбрасывается к `admin`.

Отдельный binary stable `0.3.1` относится к более ранней bootstrap-модели, поэтому при его эксплуатации следует использовать документацию именно этого бинарного выпуска.

## Документация

- [`ARCHITECTURE.md`](ARCHITECTURE.md) — целевая архитектура и обязательные продуктовые инварианты;
- [`ROADMAP.md`](ROADMAP.md) — опубликованная линия и последующие capability stages;
- [`docs/REQUIREMENTS_RU.md`](docs/REQUIREMENTS_RU.md) — каталог принятых требований;
- [`docs/RELEASE_0.16.0_RU.md`](docs/RELEASE_0.16.0_RU.md) — состав текущего опубликованного исходного релиза.

Функции более новых версий считаются предварительными до официальной публикации соответствующего релиза и не должны описываться как уже доступные.