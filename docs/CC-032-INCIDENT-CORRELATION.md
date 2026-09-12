# Control Center 0.32 — строгая корреляция incident signals

Статус: source-only / runner-free preparation. Эта часть относится только к закреплённому scope **0.32.0 Health / Incidents / Audit / Reports** и не меняет Public Stable 0.30.0.

## Цель

До появления любого автоматического incident workflow система должна уметь безопасно ответить на узкий вопрос: относится ли новый уже проверенный signal к одному существующему incident или однозначного соответствия нет.

Корреляция намеренно не использует fuzzy matching, LLM, свободный текст summary или скрытые эвристики. Exact match требует одновременно:

- одинаковый server-side `scope_id`;
- incident в состоянии `open` или `acknowledged`;
- точное совпадение множества affected resources (`kind + id + scope_id`) независимо от порядка;
- существующий signal того же `kind + source`;
- новый signal не предшествует `started_at` incident.

Resolved incident является историческим и не поглощает новый signal. Если один и тот же `signal_id` уже принадлежит incident, результат `already-linked` позволяет безопасно обработать повтор доставки без создания второй связи.

## Bounded repository lookup

`CorrelationService` не получает от вызывающей стороны произвольный список incident. После валидации observation он сам формирует bounded query только по активным `open/acknowledged` incident того же scope и по одному детерминированно выбранному exact affected-resource anchor. Максимальный размер запроса ограничен штатным `MaxListLimit`.

Если storage сообщает `HasMore`, возвращает continuation cursor или выдаёт больше запрошенного bound, решение не строится: correlation завершается fail-closed как incomplete candidate set. Это запрещает ложный `new-candidate` или случайный match на основании только первой страницы.

## Fail-closed границы

- более одного exact open/acknowledged match → `blocked / multiple_exact_open_incidents`;
- один `signal_id`, обнаруженный более чем в одном incident того же scope → `blocked / signal_id_linked_to_multiple_incidents`;
- invalid stored incident → ошибка dependency validation; такой объект нельзя тихо пропустить и затем ошибочно объявить `new-candidate`;
- unavailable/truncated repository lookup → dependency failure, а не частичный correlation result;
- вход и candidate set ограничены; неограниченный скан не является частью контракта;
- fingerprint строится только из contract version, scope, signal kind/source и canonical affected-resource identities; summary/evidence payload в digest не входит.

## Authority boundary

`incidents.correlation-decision/v1` всегда возвращает:

- `execution_authorized=false`;
- `production_mutation_allowed=false`.

Результат **не создаёт incident, не добавляет signal, не acknowledge/resolve, не меняет RBAC и не выдаёт Job/execution authority**. Persistence/creation остаются отдельной серверной mutation boundary с собственными RBAC, optimistic-concurrency, Audit и recovery требованиями.

Это также не реализует будущую policy-based automatic incident creation для Support Gateway: такая capability относится к более позднему roadmap и не должна быть неявно протащена в 0.32.

## Подготовленное покрытие

Source tests фиксируют:

1. deterministic exact match и binding к exact incident resource version/generation;
2. независимость fingerprint от порядка resources;
3. fail-closed ambiguity при двух exact active incidents;
4. replay-safe `already-linked` для известного signal ID;
5. блокировку conflicting ownership одного signal ID;
6. запрет поглощать новый signal resolved incident;
7. отсутствие match при другом resource set;
8. fail-closed invalid stored incident/invalid observation;
9. bounded candidate set;
10. deterministic active-scope repository query;
11. fail-closed truncated/unavailable repository lookup;
12. validation-before-storage для malformed observation.

Hosted qualification этим source-only проходом намеренно не запускается. Exact-head runner qualification должна выполняться отдельным runner-потоком после интеграции с текущим 0.32 incident stack.
