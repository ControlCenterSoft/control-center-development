# Дорожная карта Control Center

Статус на 10.09.2026: исходный код находится на версии **0.20.0**; последний подтверждённый официальный GitHub Release на момент этой синхронизации — **0.19.0**; отдельный stable-binary канал — **0.3.1**.

Версионная доступность определяется фактически опубликованным релизом. Архитектурные планы и кандидаты не должны описываться как уже доступные пользователю возможности.

## Опубликованная линия

- **0.6.0** — Site Autonomy и Network Foundation.
- **0.7.0** — Capacity forecast и what-if.
- **0.8.0** — advisory Placement Advice.
- **0.9.0** — Bottleneck Report.
- **0.10.0** — Capacity Horizon.
- **0.11.0** — Capacity Calibration.
- **0.12.0** — Forecast Correction.
- **0.13.0** — Calibration Trend.
- **0.14.0** — benchmark-backed Workload Profile.
- **0.15.0** — bounded nonlinear Workload Curve.
- **0.16.0** — Workload Curve Efficiency.
- **0.17.0** — Workload Scale Scenario / Scale Options.
- **0.18.0** — Placement Safety Margin.
- **0.19.0** — Placement Resource Headroom.

Capacity-возможности этой линии являются advisory-only: они формируют проверяемые рекомендации/evidence, но сами не разрешают placement или production mutation.

## Текущий следующий пакет

**0.20.0 — Placement Resource Headroom Freshness Gate.** Пакет проверяет, можно ли повторно использовать ранее рассчитанный headroom envelope с учётом точного lineage, возраста observation, индивидуальных freshness budgets, типа evidence и confidence. Состояния `current`, `degraded-evidence` и `stale` обрабатываются fail-closed; при недостаточной свежести требуется собрать новые evidence/telemetry.

До отдельной официальной публикации версия 0.20.0 не должна описываться как опубликованный релиз.

Следующие Capacity-пакеты могут расширять freshness/revalidation и policy evidence, сохраняя deterministic, fail-closed и advisory-only границы до появления отдельно подтверждённого policy/approval execution слоя.

## Core

Core включает Identity/RBAC, Desired/Actual State, Changes/Jobs, Node/Role/Lifecycle, Network, Monitoring/Health, Audit, Backup/Recovery contracts, Capacity Planner foundation и системные API.

Single-node остаётся самостоятельным способом использования. Multi-node/HA, lifecycle и recovery развиваются как first-class подсистемы; HA считается поддержанным только после фактических failure/recovery tests и официальной публикации соответствующей возможности.

Network Management поддерживает целевую модель multi-NIC, WAN/LAN, VLAN/bonding, routing, DNS/NTP, firewall, явный NAT/port-forwarding и staged changes с connectivity verification и rollback.

## Market

Market использует единый module lifecycle:

`Install → Configure → Health → Update → Migrate/Drain → Backup → Restore → Remove`

Failover добавляется только для provider, где он действительно поддержан. Основные направления: Directory Services (включая Samba AD/FreeIPA там, где применимо), DNS/DHCP, PXE Windows/Linux, Software Automation Windows/Linux, Inventory/Compliance, File Services, Monitoring и другие инфраструктурные providers.

## Lifecycle и Recovery

Управляемые операции узла включают enrollment, active, maintenance, drain, replacement и decommission/remove. Stateful workload нельзя переносить как stateless сервис: нужен provider-specific migration/recovery adapter.

Recovery охватывает Recovery Points, backup metadata/integrity, restore, PostgreSQL backup/WAL/PITR для поддерживаемых профилей, object-level recovery, RPO/RTO и restore drills. Backup без проверенного restore не считается доказанной готовностью.

## Authentication policy

После чистой установки создаётся `admin/admin`, и первый вход обязан завершиться сменой пароля. До смены обычная работа запрещена. Обновление не сбрасывает уже установленный пароль.

## Критерий публикации

Новая версия может считаться опубликованной только когда её фактический состав, schema/API contracts, security/failure tests, upgrade/migration path и release notes согласованы, обязательные проверки завершены, а опубликованный release identity однозначен.
