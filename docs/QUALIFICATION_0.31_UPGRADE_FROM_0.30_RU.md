# Control Center 0.31 — qualification обновления с Public Stable 0.30

## Назначение

Этот документ фиксирует отдельный release gate для ближайшей линии 0.31.0: безопасное обновление с фактически опубликованного Public Stable 0.30.0 на текущую committed-схему 0.31 без сброса пользовательских данных, настроек и выбранного пароля администратора.

Документ не объявляет 0.31.0 Release Candidate или Stable и не разрешает production deployment. Итоговый release status определяется только каноническим Roadmap и полным набором обязательных release gates.

## Фактическая база

Canonical Public Stable: `0.30.0`.

Опубликованный Stable 0.30 содержит SQL migration boundary `0001`–`0010`. Миграция `0011_job_timeline` в Stable 0.30 отсутствует и является первым новым schema slice текущей линии 0.31.

Published migrations `0001`–`0010` считаются immutable. Их нельзя переписывать, переименовывать или подменять при подготовке 0.31. Любое последующее изменение схемы должно оформляться новой migration с новым ordinal.

## Проверяемый переход

Новый qualification test `TestPostgresUpgradeFrom030PreservesCredentialsAndDurableJob` должен выполняться на одноразовой PostgreSQL-базе и проверять последовательность:

1. чистая установка точного Stable 0.30 migration boundary `0001`–`0010`;
2. создание локального администратора и замена bootstrap-пароля на пользовательский;
3. фиксация существующих immutable revision, Change и durable Job до обновления;
4. применение `0011_job_timeline.up.sql`;
5. проверка сохранности admin credentials, first-login state и существующего durable Job;
6. проверка единственного bounded baseline event `observed` для Job, существовавшего до 0.31;
7. повторное применение `0011` без дублирования evidence;
8. scoped rollback только `0011` с сохранением Stable 0.30 state;
9. повторное применение `0011`, reconnect к PostgreSQL и повторная проверка state/evidence.

## Обязательные invariants

Qualification считается успешной только при одновременном выполнении следующих условий:

- выбранный пользователем пароль `admin` не заменён bootstrap-значением;
- `password_change_required` не возвращается в `true` после обновления;
- Change/Job identity, action, status, attempt, max-attempts, version, timestamp и input fingerprint не переписываются migration;
- для существующего Job создаётся только bounded `observed` evidence с точными `job_version/status/attempt/occurred_at`;
- migration replay не создаёт второй timeline event для той же Job version;
- down migration удаляет только принадлежащий 0.31 timeline schema slice и не изменяет существующий Job или credentials;
- повторное обновление после rollback восстанавливает корректный baseline evidence;
- reconnect/restart PostgreSQL не меняет результат;
- никакие raw input/output, credentials, lease token, worker identity или raw error не добавляются в timeline evidence.

## Runner boundary

Этот slice подготовлен в `work/**` без создания PR и без изменения `main`, `release/**`, `develop/**` или `migration/**`. Public CI запускается на push только для этих защищённых release/development веток и на `pull_request`, поэтому work-branch используется исключительно как runner-free handoff.

В рамках non-runner потока запрещено создавать PR, выполнять `workflow_dispatch`, rerun или иным способом инициировать GitHub-hosted checks.

Отдельный runner-поток после сведения актуального 0.31 head должен выполнить один полезный exact-head qualification pass. Дублирующие workflow и synthetic load не требуются.

## Что этот gate не закрывает

Даже при PASS этого теста до 0.31 RC остаются другие независимые требования Roadmap: policy-gated cancel/retry/reconnect integration, recovery/E2E, packaging/install path, полный security/commercial review и финальная release qualification. Этот документ подтверждает только upgrade-preservation boundary с фактического Public Stable 0.30.
