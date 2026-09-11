# Control Center 0.29.0 — Product Web Shell / Overview v2

Статус: **release candidate**. Базовая версия Public Stable — **0.28.0**. Публикация допускается только после успешной qualification точного итогового SHA и последующих canonical release/Public Stable gates.

## Основное изменение

Control Center 0.29.0 открывает первый крупный этап нового Web UI: единый Product Web Shell и Overview v2. Интерфейс формирует безопасную базу для дальнейших экранов Sites/Nodes, Changes/Jobs, Health/Audit и Security, не выдавая отсутствующие либо неподтверждённые данные за нормальное состояние.

## Что добавлено

- новый активный Overview v2 как единая точка входа в продуктовый Web UI;
- контексты «Установка / Сайт / Узел» с безопасным состоянием до подключения authoritative backend data;
- явные семантики среды, актуальности, риска, состояния и уведомлений;
- базовая локаль `ru-RU` и стабильные `data-i18n` markers для последующей локализации;
- responsive layout для узких экранов;
- accessibility markers, skip-link, текстовые признаки состояния и отсутствие зависимости только от цвета;
- HTML escaping для identity-derived значений в пользовательском интерфейсе.

## Информационная безопасность

- неизвестное, не загруженное или неподтверждённое состояние отображается как неизвестное и не подменяется `Healthy`;
- новый shell не предоставляет execution authority и не разрешает production mutation;
- выбор контекста в этом scope не запускает инфраструктурные изменения;
- пользовательские identity-derived значения выводятся через безопасный HTML template escaping;
- статус и риск имеют текстовую семантику, поэтому цвет не является единственным каналом передачи критичной информации;
- существующие требования Identity/RBAC, session security, Audit и fail-closed поведения не ослабляются.

## Коммерческая и лицензионная граница

Scope 0.29.0 не добавляет сторонние runtime-компоненты и не меняет dependency graph продукта. Из-за этого изменения не возникает новых обязательств по redistribution, source-offer или NOTICE.

Существующие требования к license/SPDX evidence, authoritative source, distribution mode, commercial/redistribution disposition и release provenance сохраняются без ослабления. Release notes не заменяют машинно проверяемые qualification/release evidence.

## Совместимость, установка и обновление

Изменение является аддитивным на уровне Web UI и не добавляет SQL migration, не изменяет пользовательские данные и не требует сброса настроек или credentials. Установленный пользователем пароль `admin` при обновлении должен сохраняться; обновление не должно возвращать аккаунт к первоначальному `admin/admin`.

Обновление 0.28.0 → 0.29.0 должно пройти штатную supported-upgrade qualification. Чистая установка должна пройти тот же canonical install path, что и другие поддерживаемые релизы. Опубликованные ранее миграции остаются immutable byte-for-byte.

## Проверки для qualification

Перед выпуском точный итоговый SHA обязан подтвердить как минимум:

- public repository safety boundary;
- formatting и `go vet`;
- unit/contract tests нового Web Shell, включая fail-closed status semantics и output escaping;
- build;
- PostgreSQL 15/16/17/18 clean-install;
- PostgreSQL 15/16/17/18 supported-upgrade;
- PostgreSQL adapter/restart recovery qualification;
- race detector и restart tests;
- отсутствие переноса PASS от предыдущего SHA после изменения release identity или metadata.

## Packaging и публикация

Зелёный PR CI сам по себе не является Public Stable release. После qualification точного candidate SHA требуются merge в canonical `main`, повторная main qualification, официальный tag/source release и отдельная Public Stable promotion с предусмотренными binary/source artifacts, SHA-256 checksums, qualification/release manifests и provenance.

Дополнительные внешние review-сигналы являются вспомогательными и не заменяют deterministic CI, security qualification или обязательные release gates.

## Граница готовности

0.29.0 считается готовым к promotion только после успешной qualification точного итогового SHA, повторной проверки после canonical merge и прохождения штатных publication gates. До этого версия остаётся release candidate и не должна описываться как Public Stable.
