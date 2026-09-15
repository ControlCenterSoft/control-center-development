# Control Center Market — политика активации разработки

Статус: **CURRENT / APPROVED**  
Дата: **15.09.2026**

## 1. Решение

Market остаётся в утверждённой продуктовой последовательности **0.43–0.55**. Нумерация Market-релизов не переносится в 0.32/0.33 и правило anti-drift **Public Stable + максимум два feature-релиза** не ослабляется.

Текущий Public Stable: **0.31.1**.  
Текущее runtime code window: **0.32–0.33**.

Следовательно, Market Platform 0.43 имеет состояние:

`ARCHITECTURE_FROZEN / PREPARATION_ALLOWED / CODE_HOLD`

Это плановый admission state, а не технический blocker.

## 2. Когда начинается runtime-разработка Market

0.43 Market Platform становится `CODE_ELIGIBLE` после публикации **Public Stable 0.41.x**, потому что окно Stable+2 становится 0.42–0.43.

Далее:

| Market milestone | Scope | Минимальный опубликованный Public Stable для начала runtime-кода |
| --- | --- | --- |
| 0.43 | Market Platform v2 / Managed Provider Framework | 0.41.x |
| 0.44 | Domain Services — Samba AD / FreeIPA | 0.42.x |
| 0.45 | DNS / DHCP | 0.43.x |
| 0.46 | PXE Windows / Linux | 0.44.x |
| 0.47 | Software Automation Windows / Linux | 0.45.x |
| 0.48 | IT Asset Inventory | 0.46.x |
| 0.49–0.55 | Последующие Market modules | milestone должен находиться в пределах Stable+2 |

Gate открывается только по фактически опубликованному Public Stable. SOURCE RELEASE, RC, merged code или тестовая установка Stable не заменяют.

## 3. Что разрешено до открытия code window

Preparation Track разрешает:

- requirements и acceptance criteria;
- ADR/design;
- manifest/schema contracts;
- API/RBAC/permission contracts;
- compatibility/support matrices;
- threat/failure model;
- test specifications;
- migration/rollback/recovery design;
- операторскую и продуктовую документацию без ложных availability claims.

До открытия окна не выполняются:

- новый runtime implementation дальнего milestone;
- квалификация такого implementation как release candidate;
- release artifacts;
- пользовательские claims о доступности capability.

## 4. Ранее созданная 0.43 foundation-работа

Существующие commits, PR и issues 0.43 не удаляются: они остаются исторической трассировкой и research evidence. Они не являются исключением из Stable+2.

После открытия окна 0.43 работа возобновляется от актуального `main`; старые изменения переоцениваются и при необходимости перерабатываются. Проверки и PASS не переносятся между разными SHA.

## 5. Почему последовательность 0.44–0.48 не перенумерована

Проверены канонические Market-документы по Domain Services, PXE, Software Automation и Inventory.

- Domain Services может использовать уже существующие DNS/NTP services; управляемый DNS/DHCP 0.45 не является обязательным условием для начала 0.44.
- PXE может работать с точной target identity; будущая интеграция с Inventory/Automation улучшает lifecycle, но не является обязательным условием foundation-релиза.
- Software Automation допускает точный список targets, поэтому полный fleet Inventory 0.48 не является жёсткой зависимостью 0.47.
- Inventory remediation зависит от Software Automation, что соответствует текущей последовательности 0.47 → 0.48.

Перенумерация дала бы большой documentation/release churn без достаточной инженерной выгоды.

## 6. Automation semantics

`CODE_HOLD_BY_WINDOW` не является stop-factor. Рабочая Market-задача до открытия окна:

1. выполняет только разрешённый Preparation Track, если есть независимая подготовительная работа;
2. иначе фиксирует `NO-OP / WAITING_FOR_RELEASE_WINDOW`;
3. не требует уведомления владельца.

Настоящий blocker возникает только после открытия соответствующего release window, если работа не может продолжаться из-за ошибки, конфликтующего решения, security/recovery failure или другого вопроса, требующего решения владельца.

## 7. Источники истины

- Google Drive: `00 — Control Center — ОСНОВНОЙ ROADMAP — CANONICAL CURRENT`, CC-RM-1.25.
- Google Drive: документационный аудит, карта активных работ и матрица трассируемости.
- GitHub: `ROADMAP.md` и этот документ.
- Операторские Market-документы 14–18 описывают целевую модель, но availability определяется только опубликованным и квалифицированным релизом.
