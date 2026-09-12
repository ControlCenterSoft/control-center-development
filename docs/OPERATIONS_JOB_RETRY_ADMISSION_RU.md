# Control Center 0.31 — безопасный admission ручного retry Job

## Назначение

В интерфейсе Changes / Jobs действие «Повторить» не должно означать безусловный повтор уже завершившейся операции. Между просмотром Job и подтверждением оператором его durable state, policy, retry history и approval evidence могут измениться. Повтор старого решения без повторной проверки создаёт stale-view mutation и может повторно применить уже небезопасную операцию.

Для 0.31 подготовлен bounded контракт `ui.operations-job-retry-admission/v1`. Он формирует read-only evidence для решения о ручном retry и связывает его с:

- точным `job_id` и положительной durable `job_version`;
- исходным `change_id` и `action_name`;
- точной immutable Change revision и её SHA-256 digest;
- точным policy id/digest;
- SHA-256 digest и временем наблюдения durable manual-retry history;
- текущим manual-retry budget;
- требованием свежего approval evidence, если это предписано policy;
- единым временем наблюдения admission evidence.

## Fail-closed правила

Ручной retry может получить состояние `eligible` только когда одновременно выполнены условия:

1. операторская версия Job совпадает с фактической durable версией;
2. исходный Job находится в `failed`;
3. автоматический retry budget исходного Job уже исчерпан;
4. terminal Job не содержит активной lease;
5. policy явно разрешает manual retry;
6. durable retry-history snapshot имеет канонический SHA-256 digest;
7. policy и retry-history evidence не старше terminal состояния source Job;
8. manual-retry budget положительный и ещё не исчерпан;
9. если policy требует нового approval, передан корректный SHA-256 digest approval evidence;
10. Job/policy/retry-history evidence не новее общего `observed_at`.

Несовпадение версии возвращает `job.ErrVersionConflict` и не формирует admission. Отсутствующий/некорректный retry-history digest, устаревший history/policy snapshot, повреждённое или внутренне противоречивое состояние считаются ошибкой evidence и также работают fail-closed.

Для корректного, но неразрешённого состояния evidence возвращает `blocked` и один или несколько bounded blocker codes:

- `source_not_failed`;
- `policy_denies_retry`;
- `manual_retry_budget_exhausted`;
- `fresh_approval_required`.

Blockers канонически сортируются, а `admission_id` является детерминированным SHA-256 от полного безопасного evidence без самого `admission_id`. В digest входят retry-history digest и время его наблюдения, поэтому evidence нельзя незаметно переиспользовать с другой историей ручных повторов.

## ИБ и приватность

Admission evidence специально не содержит:

- Job input;
- `LastError` и сырые error payload;
- output/details;
- idempotency key;
- lease token/worker identity;
- credentials, secrets или authorization tokens.

`eligible` **не является execution authority**. Этот контракт не меняет Job, не создаёт новый Job, не сбрасывает attempt counter, не вызывает worker и не выполняет инфраструктурную операцию.

## Требование к будущей mutation-интеграции

Будущий endpoint ручного retry обязан перед любой mutation:

1. повторно прочитать source Job;
2. потребовать exact version precondition (предпочтительно `If-Match`);
3. повторно проверить актуальный policy/approval evidence;
4. повторно получить durable manual-retry history и подтвердить тот же либо более новый history digest/budget;
5. создать **новую retry lineage**, а не переиспользовать terminal Job и его старую lease/idempotency identity;
6. атомарно учесть manual-retry budget вместе с созданием новой lineage, чтобы параллельные запросы не превысили policy cap;
7. записать bounded Audit evidence о решении и связи с source Job;
8. не выполнять автоматический retry при version/policy/history conflict;
9. отдельно пройти race/PostgreSQL/restart/reconnect/negative-path qualification.

До этой интеграции текущий slice является source-only подготовкой и не закрывает release gate 0.31.0.

## Release boundary

Изменение относится только к Control Center 0.31.0 — ближайшему COMMITTED релизу поверх Public Stable 0.30.0. Оно не переносит функциональность из 0.32/0.33 и не проектирует возможности за разрешённой трёхрелизной границей.

SQL migration этим slice не требуется: durable retry mutation и хранение manual-retry lineage намеренно не добавляются до отдельной квалифицированной интеграции.
