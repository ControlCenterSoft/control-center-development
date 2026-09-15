# Control Center — продуктовая дорожная карта и критерии готовности

Статус: **CURRENT / SOURCE OF TRUTH FOR DEVELOPMENT SEQUENCE**  
Синхронизация: **CC-RM-1.26**  
Дата: **15.09.2026**

**ROADMAP ИЗМЕНЁН — CC-RM-1.26.** Версии Market не перенумерованы. Устранён конфликт между ранней 0.43-разработкой, правилом `Public Stable +2` и зависимостью provider-модулей от Market foundation.

## 1. Текущий релизный статус

Текущий опубликованный Public Stable — **0.31.1**. Corrective patch исправляет package/install boundary 0.31.0 без расширения feature scope. Immutable release identities 0.31.0/0.31.1 не переписываются.

Ближайшая линия — **0.32.0 Health / Incidents / Audit / Reports**. Вторая разрешённая линия — **0.33.0 Identity / RBAC / Session / Security Settings UI**.

### Динамическое окно разработки

Product code и feature-scope могут продвигаться не более чем на **два последовательных feature-релиза после текущего Public Stable**.

При Stable 0.31.1 разрешённое окно — **0.32–0.33**. 0.34+ до продвижения Stable допускают roadmap/design/preparation, но не новый product runtime code, runner qualification или release promotion.

Ближайший релиз N+1 всегда приоритетнее N+2.

## 2. Неизменяемые правила

Канонический путь изменения состояния:

`Identity/RBAC → Desired State / Change → exact-revision Approval → durable Job → typed execution → Actual State → post-condition verification → Audit/Evidence → rollback/recovery`

Обязательные invariants:

- arbitrary shell/exec не является универсальным product API;
- Unknown/Stale/Degraded не отображаются как Healthy;
- false Success является release blocker;
- WAN+LAN не включает routing/NAT автоматически;
- backup без verified restore не считается доказанной защитой;
- HA без failure/recovery qualification не считается поддержанным;
- released SQL migrations immutable byte-for-byte;
- stateful workload перемещается только через provider-specific migration/recovery semantics;
- чистая установка создаёт `admin/admin`, первый вход требует обязательной смены пароля, update не сбрасывает пользовательский пароль.

## 3. WIP, CODE_ONLY и RUNNER_ONLY

Перед началом работы проверяются active/queued jobs/runs, PR, ветки, issues и exact SHA. Уже выполняемая или завершённая работа не дублируется и не отменяется.

- `CODE_ONLY` готовит изменения и exact-SHA handoff, но не запускает runner qualification/release.
- `RUNNER_ONLY` принимает зарегистрированный exact SHA и выполняет только применимые проверки/integration/release stages.
- неизменённый SHA повторно не проверяется без подтверждённой flaky/infrastructure причины;
- недоступность или необновлённость действующего тестового сервера не блокирует разработку и выпуск; обязательные system/install/upgrade/rollback/recovery/security/HA проверки выполняются в воспроизводимом изолированном контуре.

## 4. Release train 0.32–0.42

- **0.32.0 — COMMITTED.** Health / Incidents / Audit / Reports.
- **0.33.0 — COMMITTED.** Identity / RBAC / Session / Security Settings UI.
- **0.34.0 — PLANNED.** Managed Network Planning UI.
- **0.35.0 — PLANNED.** Managed Network Apply / Verify / Rollback.
- **0.36.0 — PLANNED.** Node/Agent Enrollment, Trust, Support Gateway / Support Bundle Server.
- **0.37.0 — PLANNED.** Maintenance / Drain / Replacement / Decommission.
- **0.38.0 — PLANNED.** Role Placement + Capacity integration.
- **0.39.0 — PLANNED.** Recovery Points / Backup Repository foundation.
- **0.40.0 — PLANNED.** PostgreSQL Recovery / PITR / restore verification.
- **0.41.0 — PLANNED.** Controller Membership / Quorum / DCS.
- **0.42.0 — PLANNED.** HA / Controlled Switchover / Failover.

Planning UI предшествует risk-bearing network execution. Node enrollment предшествует lifecycle automation. Recovery foundation предшествует HA.

## 5. Market foundation 0.43 — Architecture Freeze

**0.43.0 — PLANNED / ARCHITECTURE FROZEN / CODE HOLD по release horizon.**

Scope: Managed Provider Framework + Infrastructure Solutions Foundation + Intent/Synthesis/Expansion + Market Platform v2.

Frozen scope:

- [`docs/CC-043-ARCHITECTURE-FREEZE-RU.md`](docs/CC-043-ARCHITECTURE-FREEZE-RU.md)
- [`docs/CC-043-IMPLEMENTATION-PLAN-RU.md`](docs/CC-043-IMPLEMENTATION-PLAN-RU.md)
- [`docs/MARKET_ACTIVATION_POLICY_RU.md`](docs/MARKET_ACTIVATION_POLICY_RU.md)

Architecture Freeze определяет **что** строить, но не разрешает реализацию вне release horizon.

### Activation gates 0.43

- **Stable < 0.41:** `MARKET_PREPARATION_ONLY`; runtime code/runners/promotion запрещены.
- **Stable >= 0.41 и < 0.42:** разрешён ограниченный `CODE_ONLY` provider-neutral foundation 0.43 как N+2, максимум **20% квоты CC**; 0.42 имеет абсолютный приоритет; release-qualification/promotion runners 0.43 не запускаются.
- **Stable >= 0.42:** 0.43 становится N+1; разрешены полный implementation scope, отдельные runners и release qualification до 0.43 Public Stable.

Ранее созданные 0.43 runtime branches/PR/issues — **FROZEN_REFERENCE**. Они не удаляются, но не расширяются/merge/qualify до открытия gate. После открытия gate изменения переносятся выборочно на актуальную release base и заново проверяются по exact SHA. Bulk merge старого задела запрещён.

## 6. Market modules 0.44–0.55

- **0.44** Domain Services: Samba AD / FreeIPA; Microsoft AD — external connector only.
- **0.45** DNS / DHCP.
- **0.46** PXE Deployment Windows / Linux.
- **0.47** Software Automation Windows / Linux.
- **0.48** IT Asset Inventory.
- **0.49** Software Inventory & Compliance.
- **0.50** File Services.
- **0.51** Monitoring provider.
- **0.52** Backup providers.
- **0.53** Mail & Groupware.
- **0.54** 1C:Enterprise Server.
- **0.55** Secure Web Gateway / Corporate Proxy.

### Dependency gate

Для Market действует правило строже простого Stable+2:

- 0.44 Domain Services product code — **только после 0.43 Public Stable**;
- 0.45 product code — после 0.44 Public Stable;
- далее provider-specific milestone M начинает product code после Public Stable непосредственного foundation/predecessor milestone и только при нахождении M внутри общего окна Stable+2;
- N+2 может иметь Preparation Track, но не provider-specific runtime implementation.

Комплект Domain Services `15A` и DS-M0…DS-M9 может готовиться заранее как design/acceptance/test/support preparation. Наличие design docs не является release/runtime authority.

Ожидание activation gate — **плановое состояние, не stop factor**. Задача выполняет разрешённый Preparation Track либо `NO_ACTION/PREPARED`, не создавая blocker-уведомление.

## 7. Capacity / policy-driven operations 0.56–0.59

- **0.56** Capacity Intelligence v2.
- **0.57** Policy-driven Placement.
- **0.58** Controlled Automatic Rebalance.
- **0.59** Bounded Automatic Recovery.

Автоматизация разрешена только в явно заданной policy boundary и не заменяет Change/Job/Audit.

## 8. Mobile 0.60–0.61

- **0.60** Mobile v1 read-focused.
- **0.61** bounded mobile actions with server-side revalidation.

## 9. Hardening / 1.0

- **0.62** Accessibility / Localization / Security / Performance hardening.
- **0.63** Install / Upgrade / Rollback / Migration certification.
- **0.64** Scale certification.
- **0.65** HA/DR disaster drills.
- **0.66** Integrated Production Readiness.
- **0.90** Feature Freeze.
- **0.95** Release Candidate.
- **1.0.0** Public Stable target for known scope.

## 10. Definition of Done

Capability готова только при наличии применимых:

1. object/data/API contract;
2. RBAC permissions/scopes;
3. Desired/Actual semantics;
4. failure/recovery model;
5. stale/idempotency protection;
6. health/observability/Audit;
7. positive/failure/security tests;
8. upgrade/migration path;
9. backup/restore semantics для stateful data;
10. user/operations documentation;
11. фактического соответствия заявленному поведению.

Risk-bearing operation дополнительно требует exact target, preview/diff, blast radius, preflight, approval policy, durable Job, post-condition verification и recovery path.

## 11. Release rule

Каждый начатый release train завершается официальным Public Stable. COMMITTED/RC/SOURCE RELEASE — промежуточные состояния. Security, upgrade, rollback, recovery, data-preservation и false-success gates не обходятся.

Готовый проверенный релиз публикуется в соответствующий stable repository независимо от состояния постоянного тестового сервера; установка тестового контура — отдельная эксплуатационная очередь.

## 12. Product/documentation boundary

Control Center — самостоятельный infrastructure control plane. Публичная продуктовая документация не раскрывает внутреннюю методологию разработки/CI, служебную инфраструктуру, внутренние адреса, секреты, рабочие репозитории/ветки или другие внутренние данные, не требующиеся пользователю продукта.

Полный канонический roadmap и журнал изменений ведутся в Google Drive; этот файл является синхронизированным repository-side development view.
