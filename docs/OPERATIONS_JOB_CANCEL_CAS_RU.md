# Control Center 0.31 — безопасная отмена Job по точной версии

## Назначение

Для операторской модели Changes / Jobs отмена должна относиться к тому состоянию Job, которое оператор фактически просмотрел. Между чтением карточки Job и нажатием «Отменить» Job может быть захвачен worker-ом, перейти в retry, завершиться или получить новую durable-версию.

Поэтому подготовлен отдельный optimistic-concurrency контракт `VersionedCancellationRepository`: отмена принимает `job_id` и точный `expected_version`.

## Fail-closed поведение

Если фактическая версия Job отличается от просмотренной, операция возвращает `ErrVersionConflict` и **не изменяет** durable Job.

При совпадении версии сохраняются текущие правила состояния:

- `queued` / `retry_wait` переходят в `cancelled`;
- `running` переходит в `cancel_requested`, действующая lease сохраняется для корректного завершения worker boundary;
- terminal Job при совпадающей текущей версии возвращается без повторной мутации;
- неизвестный Job возвращает `ErrNotFound`.

PostgreSQL-вариант выполняет проверку версии непосредственно в `UPDATE ... WHERE id=? AND version=?`, то есть проверка и переход образуют одну CAS-операцию. Memory repository реализует ту же семантику под одной блокировкой.

## Security boundary

Контракт устраняет stale-view mutation: старая карточка Job больше не может считаться достаточным основанием для отмены уже изменившегося задания.

Сам контракт не выдаёт новых permission, не ослабляет `orchestration.jobs.cancel`, не создаёт execution authority, не меняет retry policy, не запускает Job и не содержит credentials/секретов.

## Граница текущего runner-free slice

Этот source slice намеренно **не переключает production HTTP endpoint** на новый контракт и не объявляет release gate закрытым. Существующий runtime-путь `POST /api/v1/jobs/{jobId}/cancel` остаётся без изменения до отдельной квалифицированной интеграции.

Следующий runner-зависимый интеграционный шаг должен:

1. потребовать от mutation endpoint точный version precondition (рекомендуемый HTTP-контракт — `If-Match` с положительной durable Job version);
2. вызывать только `RequestCancelIfVersion` для операторской отмены;
3. отображать version conflict как безопасный conflict/precondition response без автоматического retry;
4. проверить PostgreSQL и memory семантику, RBAC `orchestration.jobs.cancel`, race claim/cancel, running lease, terminal idempotency и reconnect/read-model flow;
5. после любого изменения exact head заново пройти штатные release/security qualification gates.

Изменение относится к scope 0.31.0. Новая SQL migration не требуется: используется существующее durable поле `cc_jobs.version`.
