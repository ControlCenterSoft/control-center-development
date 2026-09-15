# Control Center — политика активации Market

Статус: **CURRENT / DEVELOPMENT CONTROL POLICY**  
Roadmap baseline: **CC-RM-1.26**  
Дата: **15.09.2026**

## 1. Решение

Market не переносится в 0.32/0.33 и не получает исключение из правила release horizon.

Общее правило сохраняется: product code может продвигаться не дальше чем на два последовательных feature-релиза после текущего Public Stable. При Public Stable 0.31.1 разрешённое окно product code — 0.32.0–0.33.0.

Для Market поверх этого правила действует дополнительная dependency-gate: provider-specific module не переходит в product code, пока не опубликован Public Stable foundation, от которого он зависит.

## 2. Текущий режим

При Public Stable **0.31.1** статус Market: **MARKET_PREPARATION_ONLY**.

Разрешены:
- requirements и ADR/архитектура;
- acceptance criteria и Definition of Done;
- compatibility/support matrices;
- test, migration, rollback и recovery design;
- API/schema/permission design как документация;
- UX flows и документация без runtime claims.

Не разрешены до открытия gate:
- новый runtime product code дальнего Market milestone;
- merge такого кода в canonical main;
- runner qualification/release qualification такого кода;
- release artifacts/promotion;
- пользовательские claims о готовой Market capability.

## 3. Фаза M1 — ранний старт 0.43

Trigger: **Public Stable >= 0.41.x**.

После этого 0.43 входит в окно Stable+2 и разрешается ограниченный **CODE_ONLY** provider-neutral foundation, который не зависит от незавершённого 0.42.

Ограничения M1:
- максимум **20% квоты Control Center** на 0.43; при общей квоте CC=60% это максимум 12% общего development budget;
- 0.42 остаётся абсолютным приоритетом;
- release-qualification/promotion runners для 0.43 не запускаются;
- нельзя делать provider-specific 0.44+ product code;
- результат передаётся как exact SHA только после открытия соответствующего runner gate.

## 4. Фаза M2 — полный запуск 0.43

Trigger: **Public Stable >= 0.42.x**.

0.43 становится N+1. Разрешены полный implementation scope, отдельные runner qualification tasks и продвижение к **0.43 Public Stable**.

Недоступность, необновлённость или отставание действующего тестового сервера не является stop factor. Обязательные system/install/upgrade/rollback/recovery/security/HA проверки выполняются в воспроизводимом изолированном контуре.

## 5. Фаза M3 — Market modules

**0.44 Domain Services product code запускается только после 0.43 Public Stable.**

Это ограничение строже простого Stable+2: Domain Services зависит от Managed Provider Framework / Infrastructure Solutions Foundation 0.43 и не должен строиться против нестабильного foundation.

Комплект 15A, DS-M0…DS-M9, test plans, compatibility/support matrix и архитектурная подготовка могут продолжаться заранее как Preparation Track.

Для 0.45–0.55 действует тот же принцип:
- N+2 может иметь Preparation Track;
- provider-specific product code milestone M начинается после Public Stable непосредственного foundation/predecessor milestone и только если M находится внутри общего окна Stable+2.

## 6. Ранее созданный Market задел

Все ранее созданные Market runtime branches/PR/issues получают статус **FROZEN_REFERENCE** до открытия соответствующего gate.

Правила:
- не удалять накопленный задел;
- не расширять его новым runtime scope;
- не запускать повторную runner qualification;
- не merge/promote только потому, что код уже существует;
- после открытия gate переносить изменения выборочно через rebase/cherry-pick на актуальную release base;
- полностью повторять review/qualification для нового exact SHA;
- bulk merge старого Market задела запрещён.

## 7. Поведение плановых задач

Ожидание Market activation gate — **не стоп-фактор** и не требует пользовательского уведомления.

До открытия gate Market-задача должна:
1. проверить текущий Public Stable и active/queued work;
2. выполнить только разрешённый Preparation Track, если есть независимая работа;
3. иначе завершиться `NO_ACTION/PREPARED`;
4. не запускать runners и не создавать дублирующую работу.

Реальный stop factor — только фактический дефект, конфликт, security/recovery blocker или решение, которое невозможно принять без участия владельца продукта.

## 8. Таблица активации

| Milestone | Режим | Trigger |
|---|---|---|
| 0.43 | Preparation only | Stable < 0.41 |
| 0.43 | CODE_ONLY foundation, <=20% CC quota, без release runners | Stable >= 0.41 и < 0.42 |
| 0.43 | Full code + runners + qualification | Stable >= 0.42 |
| 0.44 Domain Services | Preparation only | до 0.43 Public Stable |
| 0.44 Domain Services | Product code + qualification по DS-M0…DS-M9 | после 0.43 Public Stable |
| 0.45–0.55 | Preparation N+2; product code после predecessor Stable | по dependency gate + Stable+2 |

## 9. Source of truth

При конфликте приоритет имеют:
1. Google Drive `00 — Control Center — ОСНОВНОЙ ROADMAP — CANONICAL CURRENT`, CC-RM-1.26;
2. этот policy-файл для операционного применения Market gate;
3. `docs/CC-043-ARCHITECTURE-FREEZE-RU.md` и `docs/CC-043-IMPLEMENTATION-PLAN-RU.md` для frozen scope 0.43;
4. комплект 15A для Domain Services 0.44.

Architecture Freeze определяет **что** реализовывать, но не является разрешением реализовывать это вне release horizon.
