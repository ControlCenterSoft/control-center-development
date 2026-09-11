# Control Center 0.30.0 — Sites / Nodes / Inventory UI

Статус: **release candidate**. Базовая версия Public Stable — **0.29.0**. Публикация допускается только после успешной qualification точного итогового SHA и последующих canonical release/Public Stable gates.

## Основное изменение

Control Center 0.30.0 продолжает развитие продуктового Web UI и добавляет read-only представление «Сайты / Узлы / Инвентарь». Интерфейс показывает фактическую инфраструктуру только в пределах доступного authoritative evidence и не подменяет неизвестное, устаревшее или недоступное состояние вымышленным благополучным результатом.

## Что добавлено

- представление площадок и узлов с адаптивной компоновкой;
- карточки узлов с ролями, capabilities, hardware и connectivity summary;
- явная актуальность данных и границы evidence для Desired State, Actual State и version skew;
- read-only provider/API boundary без побочных инфраструктурных изменений;
- JSON Schema контракта inventory read-model;
- мобильная компоновка с одной карточкой узла в строке на узких экранах;
- негативные тесты API/rendering и fail-closed сценариев.

## Информационная безопасность

- `unavailable`, `stale` и `expired` не отображаются как `Healthy` и не трактуются как подтверждённое отсутствие объектов;
- ошибки authoritative provider не раскрываются клиенту как внутренние детали;
- mutation-методы на read-only inventory boundary отклоняются;
- новый UI не предоставляет execution authority и не разрешает production mutation;
- неизвестное, неполное либо неконсистентное evidence обрабатывается fail-closed;
- существующие требования Identity/RBAC, session security, Audit, network safety, recovery и staged changes не ослабляются.

## Коммерческая и лицензионная граница

Scope 0.30.0 не добавляет сторонние runtime-компоненты, SQL-миграции или новые внешние зависимости и не меняет dependency graph продукта. Новых обязательств по redistribution, source-offer или NOTICE из-за этого scope не возникает.

Существующие требования к license/SPDX evidence, authoritative source, distribution mode, commercial/redistribution disposition, versioned evidence и release provenance сохраняются без ослабления. Release notes не заменяют машинно проверяемые qualification/release evidence.

## Совместимость, установка и обновление

Изменение аддитивно на уровне Web UI/read-model и не должно изменять пользовательские данные, секреты или настройки. Установленный пользователем пароль `admin` при обновлении обязан сохраняться; обновление не должно возвращать аккаунт к первоначальному `admin/admin`.

Обновление 0.29.0 → 0.30.0 должно пройти штатную supported-upgrade qualification. Чистая установка должна пройти canonical install path. Опубликованные ранее миграции должны оставаться immutable byte-for-byte. При неуспешном обновлении применяются штатные recovery/rollback требования без потери пользовательских данных и credentials.

## Проверки для qualification

Перед выпуском точный итоговый SHA обязан подтвердить как минимум:

- public repository safety boundary;
- formatting и `go vet`;
- unit/contract tests Sites / Nodes / Inventory, включая fail-closed semantics и read-only boundary;
- build;
- PostgreSQL 15/16/17/18 clean-install;
- PostgreSQL 15/16/17/18 supported-upgrade;
- PostgreSQL adapter/restart recovery qualification;
- race detector и restart tests;
- отсутствие новых непроверенных runtime dependencies;
- сохранение release/security/commercial boundaries после изменения release identity;
- отсутствие переноса PASS от предыдущего SHA после изменения VERSION, build metadata или release notes.

## Packaging и публикация

Зелёный PR CI сам по себе не является Public Stable release. После qualification точного candidate SHA требуются merge в canonical `main`, повторная main qualification, официальный tag/source release и отдельная Public Stable promotion с предусмотренными binary/source artifacts, SHA-256 checksums, qualification/release manifests и provenance.

Публикация должна использовать только canonical release artifacts и не подменять Stable неподтверждённой сборкой. Дополнительные внешние review-сигналы являются вспомогательными и не заменяют deterministic CI, security qualification или обязательные release gates.

## Граница готовности

0.30.0 считается готовым к promotion только после успешной qualification точного итогового SHA, повторной проверки после canonical merge и прохождения штатных publication gates. До этого версия остаётся release candidate и не должна описываться как Public Stable.
