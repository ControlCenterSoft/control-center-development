# Control Center 0.30.0 — Sites / Nodes / Inventory

Статус: **разработка, не release candidate**. Базовая версия Public Stable — **0.28.0**; предыдущий последовательный release candidate — **0.29.0**. Документ фиксирует границу текущего 0.30-slice и не означает готовность к публикации.

## Основное изменение

Control Center 0.30.0 развивает Product Web Shell в сторону фактического представления инфраструктуры: Sites, Nodes и Inventory. Ключевой принцип — UI показывает только подтверждённое read-only evidence. Отсутствующий authoritative источник, неподдерживаемый legacy-контракт или невалидная проекция не подменяются пустым либо «здоровым» состоянием.

## Реализованный slice

- введён versioned read-model contract `ui.infrastructure-inventory/v1` и JSON Schema;
- добавлена deterministic Sites/Nodes проекция с аппаратной сводкой, ролями, capabilities и состоянием сетевых интерфейсов;
- актуальность наблюдений классифицируется как `current`, `stale` или `expired` с fail-closed состоянием `unavailable` для незагруженных источников;
- legacy enrollment не используется как доказательство полного 0.30 inventory: adapter принимает только явный agent enrollment v2 с hardware/site/freshness evidence;
- Desired State, Actual State и version skew до подключения соответствующих authoritative projections отображаются как `unavailable`, а не вычисляются из косвенных данных;
- добавлен read-only API boundary с валидацией provider envelope, `Cache-Control: no-store`, отказом от mutation methods и скрытием внутренних ошибок;
- API подключается только при явной передаче authoritative provider и защищается существующим `resources.read`; без provider endpoint остаётся отсутствующим;
- подготовлен русскоязычный responsive UI Sites/Nodes; на узком экране карточки узлов идут по одной в строке;
- view-модель намеренно не раскрывает серийные номера, machine/product identifiers, MAC/IP-адреса и certificate identity metadata, которые не нужны для обзорного экрана.

## Информационная безопасность

- `Unknown`/`unavailable`, `stale` и `expired` не преобразуются в `Healthy` или `Success`;
- ошибка provider, невалидный contract envelope и отсутствие источника fail-closed возвращают unavailable evidence;
- read endpoint не имеет execution authority и не может менять enrollment, topology, Desired State, сеть или host configuration;
- endpoint использует отдельное read-разрешение `resources.read`, а не permissions нормализации/reconcile/mutation;
- детали backend/provider errors не выдаются клиенту;
- HTTP response для inventory запрещает кеширование (`no-store`) и включает `nosniff`;
- sensitive inventory fields минимизированы на уровне read-model, а не только скрыты CSS/HTML;
- существующие требования Identity/RBAC, обязательной смены первоначального `admin/admin`, session security и Audit не ослабляются.

## Коммерческая и лицензионная граница

Текущий slice не добавляет сторонние runtime-библиотеки и не меняет dependency graph. Новых обязательств по redistribution, NOTICE или source-offer из-за этой части 0.30.0 не возникает.

Перед promotion всё равно должны быть повторно подтверждены machine-readable license/SPDX evidence, authoritative source identity, distribution mode, commercial/redistribution disposition и release provenance точного итогового SHA.

## Совместимость, установка и обновление

Текущий slice не содержит SQL migration и не меняет пользовательские данные, credentials или сетевую конфигурацию. Существующий пароль `admin` при обновлении не должен сбрасываться; первоначальный `admin/admin` допустим только на чистой установке с обязательной сменой при первом входе.

Upgrade-path к 0.30.0 не считается подтверждённым до qualification финального candidate SHA. Опубликованные ранее миграции должны оставаться immutable byte-for-byte.

## Что ещё не считается готовым

До release-candidate состояния необходимо как минимум:

- подключить authoritative production projection для Sites/Nodes/Inventory без обхода admission semantics agent enrollment v2;
- квалифицировать отображение Desired State / Actual State и version skew на реальных authoritative источниках либо сохранить их явно unavailable;
- включить Sites/Nodes UI в штатную authenticated navigation только после появления подтверждённого provider;
- подтвердить границы scope/RBAC для multi-site и delegated management scenarios;
- закончить accessibility/responsive и stale/expired UX qualification на точном candidate SHA;
- пройти штатные install/upgrade/restart/recovery/security/commercial release gates.

## Проверки для qualification

Перед переводом 0.30.0 в release candidate точный итоговый SHA должен подтвердить как минимум:

- formatting, `go vet`, unit/contract tests и build;
- JSON Schema / read-model compatibility;
- fail-closed tests: provider unavailable, malformed envelope, unknown Site, duplicate Node, stale/expired evidence;
- RBAC: anonymous → deny, unbound identity → deny, `resources.read` roles → read-only access;
- отсутствие mutation authority у inventory UI/API;
- privacy/minimization review состава выдаваемых inventory полей;
- PostgreSQL 15/16/17/18 clean-install и supported-upgrade;
- restart/recovery qualification authoritative projections;
- public repository safety boundary и license/SPDX/commercial evidence;
- отсутствие переноса PASS от другого SHA.

## Граница готовности

Наличие подготовленного кода само по себе не делает 0.30.0 release candidate или Stable. Promotion возможна только после завершения authoritative provider wiring, qualification точного итогового состояния и прохождения штатных release/publication gates.
