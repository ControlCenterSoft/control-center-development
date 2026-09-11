# Control Center 0.30.0 — Sites / Nodes / Inventory

Статус: **release candidate**. Базовая версия Public Stable — **0.29.0**. Публикация допускается только после успешной qualification точного итогового candidate SHA, повторной проверки canonical `main`, официального source release и отдельной Public Stable promotion.

## Основное изменение

Control Center 0.30.0 развивает Product Web Shell до фактического read-only представления инфраструктуры: Sites, Nodes и Inventory. Ключевой принцип — UI показывает только подтверждённое evidence. Отсутствующий authoritative источник, неподдерживаемый legacy-контракт или невалидная проекция не подменяются пустым либо «здоровым» состоянием.

## Что входит в candidate

- versioned read-model contract `ui.infrastructure-inventory/v1` и JSON Schema;
- deterministic Sites/Nodes projection с аппаратной сводкой, ролями, capabilities и состоянием сетевых интерфейсов;
- freshness states `current`, `stale`, `expired` и fail-closed `unavailable`;
- authoritative inventory provider boundary без использования legacy enrollment как доказательства полного inventory;
- Desired State, Actual State и version skew показываются как `unavailable`, пока соответствующее authoritative evidence отсутствует;
- read-only API boundary с проверкой provider envelope, `Cache-Control: no-store`, `X-Content-Type-Options: nosniff`, запретом mutation methods и сокрытием внутренних provider errors;
- API и UI защищены существующим `resources.read`; anonymous и unbound identities не получают доступ;
- authenticated routing Sites/Nodes включается только через подтверждённый provider boundary;
- русскоязычный responsive UI; на узком экране карточки узлов идут вертикально по одной в строке;
- sensitive inventory fields минимизированы на уровне read-model: обзорный экран не раскрывает серийные номера, machine/product identifiers, MAC/IP и certificate identity metadata.

## Информационная безопасность

- `Unknown`/`unavailable`, `stale` и `expired` не преобразуются в `Healthy` или `Success`;
- provider error, невалидный contract envelope и отсутствие источника обрабатываются fail-closed;
- inventory endpoint не имеет execution authority и не может менять enrollment, topology, Desired State, сеть или host configuration;
- endpoint использует read-разрешение `resources.read`, а не reconcile/mutation permissions;
- детали backend/provider errors не выдаются клиенту;
- response не кешируется и защищён `nosniff`;
- состав выдаваемых inventory полей минимизирован до необходимых для обзорного интерфейса;
- существующие требования Identity/RBAC, обязательной смены первоначального `admin/admin`, session security и Audit не ослабляются.

## Коммерческая и лицензионная граница

Scope 0.30.0 не добавляет сторонние runtime-библиотеки и не меняет dependency graph. Изменение не создаёт новых redistribution, NOTICE или source-offer обязательств.

Перед promotion сохраняются обязательные machine-readable license/SPDX, authoritative source identity, distribution mode, commercial/redistribution disposition и release provenance gates для точного итогового SHA.

## Совместимость, установка и обновление

0.30.0 не добавляет SQL migration и не изменяет пользовательские данные, credentials или сетевую конфигурацию. Существующий пароль `admin` при обновлении не должен сбрасываться; первоначальный `admin/admin` допустим только на чистой установке с обязательной сменой при первом входе.

Опубликованные ранее migrations остаются immutable byte-for-byte. Supported upgrade с текущего Stable 0.29.0 и clean-install считаются подтверждёнными только результатами qualification точного candidate SHA.

## Обязательные qualification gates

Точный итоговый candidate SHA должен подтвердить как минимум:

- public repository safety boundary;
- formatting, `go vet`, unit/contract tests и build;
- JSON Schema/read-model compatibility;
- fail-closed scenarios: provider unavailable, malformed envelope, unknown Site, duplicate Node, stale/expired evidence;
- RBAC: anonymous → deny, unbound identity → deny, `resources.read` → read-only access;
- отсутствие mutation authority у inventory UI/API;
- privacy/minimization границы inventory полей;
- PostgreSQL 15/16/17/18 clean-install и supported-upgrade;
- PostgreSQL adapter/restart recovery qualification;
- race detector и restart tests;
- неизменность опубликованных migrations;
- license/SPDX/commercial evidence и release provenance exact SHA;
- отсутствие переноса PASS от другого SHA.

## Packaging и публикация

Зелёный PR CI сам по себе не является Public Stable. После qualification exact candidate SHA требуются canonical merge в `main`, повторная main qualification, официальный tag/source release и отдельная Public Stable promotion с binary/source artifacts, SHA-256 checksum/sidecar, qualification/release manifests и provenance.

## Граница готовности

0.30.0 считается готовым к Public Stable только после прохождения всех обязательных qualification/publication gates на точном итоговом состоянии. До этого версия остаётся release candidate и не должна описываться как Stable.
