# Control Center 0.31 — semantic diff для Change review

## Назначение

Контракт `ui.operations-semantic-diff/v1` формирует детерминированное read-only evidence для сравнения двух immutable configuration revisions перед approval. Он относится к закреплённому scope Control Center 0.31.0 и сам по себе не разрешает запуск Job, не меняет Change state и не создаёт обход RBAC/approval policy.

Контракт связывает результат с точными `base_revision_id`, `target_revision_id`, sequence и SHA-256 digest обеих revisions. Target обязан быть более новой последовательной ревизией относительно base; сравнение одинакового revision ID отклоняется fail-closed.

## Что получает интерфейс

Результат содержит только структурные изменения:

- `added` — путь появился;
- `removed` — путь исчез;
- `changed` — значение по существующему пути изменилось.

Каждая запись содержит JSON Pointer path и тип изменения. Значения до/после намеренно не публикуются: configuration revisions могут содержать credentials, токены и другие чувствительные данные.

## Безопасность и ограничения

- входные revisions должны быть JSON objects;
- сравнение привязано к точным immutable revision identity и digest;
- target sequence должен быть строго больше base sequence;
- максимум 256 diff entries;
- максимум 2048 символов в path;
- максимальная глубина сравнения — 64 уровня;
- при превышении любого лимита diff не обрезается, а отклоняется целиком;
- JSON Pointer segments экранируются по RFC 6901;
- массивы и другие не-object значения сравниваются атомарно;
- значения конфигурации в контракт не попадают.

Fail-closed поведение обязательно: неполная identity, неканонический digest, trailing JSON, чрезмерная глубина или размер review surface являются ошибкой, а не основанием показать частичный либо «успешный» diff.

## Граница интеграции

Этот slice не меняет `VERSION`, не является release candidate и не должен самостоятельно подключаться к production routing. Следующий runner-зависимый поток должен:

1. квалифицировать код и JSON Schema на exact head;
2. интегрировать evidence в authoritative Changes / Jobs provider без дублирования существующего read model;
3. проверить RBAC на просмотр semantic diff;
4. связать approval UI с exact revision/hash и запретить approval при изменившемся target revision;
5. сохранить `no-store`, fail-closed и redaction границы административного UI.

Новых сторонних runtime dependencies, SQL migrations и коммерческих redistribution obligations этот slice не добавляет.
