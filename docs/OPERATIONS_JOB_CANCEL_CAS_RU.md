# Control Center 0.31 — безопасная отмена Job по точной версии

## Назначение

Для операторской модели Changes / Jobs отмена должна относиться к тому состоянию Job, которое оператор фактически просмотрел. Между чтением карточки Job и нажатием «Отменить» Job может быть захвачен worker-ом, перейти в retry, завершиться или получить новую durable-версию.

Для этого используется optimistic-concurrency контракт `VersionedCancellationRepository`: отмена принимает `job_id` и точный `expected_version`.

## Fail-closed поведение

Если фактическая версия Job отличается от просмотренной, операция возвращает `ErrVersionConflict` и **не изменяет** durable Job.

При совпадении версии сохраняются текущие правила состояния:

- `queued` / `retry_wait` переходят в `cancelled`;
- `running` переходит в `cancel_requested`, действующая lease сохраняется для корректного завершения worker boundary;
- terminal Job при совпадающей текущей версии возвращается без повторной мутации;
- неизвестный Job возвращает `ErrNotFound`.

PostgreSQL-вариант выполняет проверку версии непосредственно в `UPDATE ... WHERE id=? AND version=?`, то есть проверка и переход образуют одну CAS-операцию. Memory repository реализует ту же семантику под одной блокировкой.

## HTTP boundary

Production runtime для `POST /api/v1/jobs/{jobId}/cancel` связывает mutation с точной durable Job version через обязательный `If-Match`.

Допускается положительная decimal version в обычной или quoted ETag-форме, например `If-Match: "3"`. Wildcard, weak ETag, список версий, нулевая и некорректная version отклоняются fail-closed.

Поведение endpoint:

- отсутствующий `If-Match` → `428 Precondition Required`;
- некорректный `If-Match` → `400 Bad Request`;
- Job отсутствует → `404 Not Found`;
- Job изменился после просмотра → `412 Precondition Failed`, без автоматического retry и без мутации;
- точная текущая version → `202 Accepted` и CAS-вызов `RequestCancelIfVersion`;
- успешный ответ возвращает `ETag` с результирующей durable Job version;
- terminal Job с точной текущей version остаётся идемпотентным.

После `412` оператор или UI обязан заново прочитать Job, проверить новое состояние/version и только затем принять новое решение. Старый operator intent не переносится автоматически на изменившуюся Job.

## Security boundary

Контракт устраняет stale-view mutation: старая карточка Job больше не может считаться достаточным основанием для отмены уже изменившегося задания.

Интеграция не выдаёт новых permission и не ослабляет `orchestration.jobs.cancel`. Authentication/RBAC выполняются до mutation; version precondition не создаёт execution authority, не меняет retry policy, не запускает Job и не содержит credentials/секретов.

Production wiring требует repository, реализующий `VersionedCancellationRepository`; отсутствие такого контракта считается ошибкой конфигурации и не должно приводить к fallback на небезопасную отмену.

## Qualification

Для HTTP-интеграции обязательны проверки:

1. missing/invalid `If-Match` не изменяет Job;
2. stale pre-claim/pre-retry version отклоняется;
3. current queued/retry version допускает отмену;
4. running Job переходит в `cancel_requested` с сохранением действующей lease;
5. terminal exact-version replay идемпотентен;
6. RBAC `orchestration.jobs.cancel` остаётся обязательным;
7. reconnect после `412` требует нового чтения/current version;
8. PostgreSQL и memory CAS-семантика совпадает;
9. exact head проходит штатные Public CI, PostgreSQL compatibility/restart, race, build и public-safety gates.

Изменение относится к scope 0.31.0. Новая SQL migration не требуется: используется существующее durable поле `cc_jobs.version`.
