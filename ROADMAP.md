# Дорожная карта Control Center

Статус на 10.09.2026: последний подтверждённый официальный исходный релиз — **0.21.0**. Отдельный стабильный бинарный канал остаётся на **0.3.1**. Эти линии имеют разные release identity и не должны смешиваться.

## Опубликованная линия

- **0.6.0** — Site Autonomy и Network Foundation.
- **0.7.0** — Capacity forecast и what-if.
- **0.8.0** — Placement Advice.
- **0.9.0** — Bottleneck Report.
- **0.10.0** — Capacity Horizon.
- **0.11.0** — Capacity Calibration.
- **0.12.0** — Forecast Correction.
- **0.13.0** — Calibration Trend.
- **0.14.0** — Workload Profile.
- **0.15.0** — Workload Curve.
- **0.16.0** — Workload Curve Efficiency.
- **0.17.0** — Workload Scale Scenario / Scale Options.
- **0.18.0** — Placement Safety Margin.
- **0.19.0** — Placement Resource Headroom.
- **0.20.0** — Placement Resource Headroom Freshness Gate.
- **0.21.0** — Placement Resource Reuse Gate.

Capacity-функции этой линии остаются advisory-only и не являются разрешением на автоматическое изменение инфраструктуры.

## Архитектурные направления

**Core**: Identity/RBAC, Desired/Actual State, Changes/Jobs, Node/Role/Lifecycle, Network, Monitoring/Health, Audit, Backup/Recovery, Capacity Planner и системные API.

**Market**: устанавливаемые инфраструктурные модули с собственными compatibility, capacity, network и recovery metadata. Основные направления включают Directory Services, DNS/DHCP, PXE, Software Automation, Inventory/Compliance, File Services и Monitoring.

**Single-node и HA**: single-node является самостоятельным режимом. Multi-node/HA считается поддержанным только для явно опубликованных и проверенных профилей.

**Lifecycle и Recovery**: модель охватывает обслуживание, вывод узла из нагрузки, замену, вывод из эксплуатации, резервное копирование и проверяемое восстановление.

**Networking**: модель учитывает multi-NIC, WAN/LAN, VLAN/bonding, routing, DNS/NTP, firewall и контролируемые сетевые изменения; NAT/port-forwarding включается только явно.

**Capacity Planner**: прогноз, what-if, bottleneck, profiles/curves, scale scenarios, safety margin, resource headroom, freshness/revalidation и reuse evidence развиваются как детерминированный рекомендательный контур.

**Authentication**: после чистой установки используется начальная локальная учётная запись `admin/admin` с обязательной сменой пароля при первом входе; обновление не сбрасывает уже установленный пароль.

## Следующий пакет

Следующий номер версии и его фактический scope фиксируются только после появления подтверждённого кандидата. Планируемая возможность не должна описываться как опубликованная до появления однозначного release identity.
