# Control Center 0.31 — ручной повтор Job: revalidation и новая lineage

## Назначение

Этот слой закрывает mutation-side границу ручного повтора для `Changes / Jobs`: решение оператора относится к **точной durable-версии завершившегося `failed` Job**, точной immutable revision Change и точному policy/retry-history evidence. Наличие ранее построенного `eligible` admission не является разрешением на mutation само по себе.

Перед созданием нового Job система повторно читает и проверяет:

- текущую durable-версию исходного Job;
- `ChangeID`, action, terminal status и attempt/max-attempts исходного Job;
- точные `revision_id` и `revision_digest`;
- точные `policy_id` и `policy_digest`;
- фактический manual-retry budget;
- необходимость и digest свежего approval evidence;
- актуальный digest retry-history.

Если любой из этих параметров изменился после просмотра оператором, mutation завершается fail-closed и требует обновить карточку/доказательства. «Почти тот же» policy, новая revision или более свежая история с другим digest не переиспользуются автоматически.

## Атомарная новая lineage

Успешная revalidation **не перезапускает старый Job**. Создаётся новый queued Job с новым `job_id` и новым idempotency key, а исходный failed Job остаётся неизменным.

В одной durable-транзакции фиксируются:

1. exact source Job и его версия;
2. новый queued retry Job;
3. immutable связь `root → source → retry`;
4. admission ID, который видел оператор;
5. новый revalidation admission ID;
6. revision/policy/retry-history digests;
7. optional approval evidence digest;
8. время запроса.

Точный повтор того же mutation-запроса после неопределённого сетевого ответа идемпотентен и возвращает уже созданную lineage. Повторное использование того же reviewed admission для другой lineage блокируется.

## Безопасность и приватность

Контракт ручного повтора не переносит в evidence:

- raw Job input/output;
- `LastError` и технические failure details;
- lease token/worker identity;
- credentials, secrets и access tokens.

Исходный Job может содержать необходимые для исполнения typed input внутри защищённого durable Job store; новый child Job наследует его только внутри атомарной repository boundary. Эти данные не становятся частью retry-admission или lineage evidence.

Ручной повтор не даёт generic command/shell authority, не claim-ит child Job, не запускает worker и не объявляет успешный результат. После создания новый Job проходит обычный typed execution path, post-condition verification, Audit/evidence и recovery правила 0.31.

## Fail-closed условия

Новая lineage не создаётся, если:

- reviewed admission повреждён или его digest не совпадает;
- reviewed admission был `blocked`;
- source Job исчез или его durable version изменилась;
- source Job не находится в terminal `failed` после исчерпания автоматических attempts;
- source Job сохраняет lease;
- revision, policy, approval или retry-history отличаются от просмотренных оператором;
- manual-retry budget исчерпан;
- policy запретил ручной повтор;
- policy требует fresh approval, а подтверждённого digest нет;
- новый Job ID/idempotency key конфликтует с другой операцией.

## Persistence

Для новой схемы используется отдельная миграция `0012_manual_job_retry_lineage.up.sql`. Ранее опубликованные migration-файлы не изменяются byte-for-byte.

Таблица `cc_job_manual_retry_lineage` хранит только bounded identifiers/digests и ссылки на durable Jobs. Это позволяет восстанавливать retry chain после рестарта и считать фактическое число ручных повторов из immutable evidence, не полагаясь на клиентский счётчик.

## Release boundary

Этот slice относится только к Control Center **0.31.0**. Он не повышает `VERSION`, не создаёт RC/Public Stable и не закрывает самостоятельно весь release gate.

После него для 0.31 всё ещё необходимы интеграция authoritative retry-history/policy providers, полное `approval → Job → verification → recovery` E2E, recovery evidence, install/upgrade/rollback qualification, packaging и финальные ИБ/коммерческие/release gates.
