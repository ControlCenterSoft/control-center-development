# Control Center 0.31 — представление Changes / Jobs

## Назначение

Контракт `ui.operations-change-job/v1` формирует безопасное read-only представление операции для интерфейса администратора. Он связывает утверждённый `Change` с durable `Job`, показывает текущее состояние выполнения и наличие результата проверки, но не создаёт новый контур исполнения и не обходит существующие RBAC, Change/Approval и Job-механизмы.

Этот слой относится к подготовке 0.31.0 и сам по себе не объявляет 0.31.0 release candidate или Stable.

## Что показывается оператору

Для `Change` доступны:

- идентификатор, действие, инициатор и immutable revision;
- риск и текущее состояние;
- число записанных и требуемых approvals;
- вычисленный признак `approvals_satisfied`;
- версия и время последнего изменения.

Для связанного `Job` доступны только безопасные эксплуатационные метаданные:

- идентификатор и статус;
- текущая попытка и предел попыток;
- версия;
- время создания и последнего изменения.

Сырые input-параметры Job, idempotency key, lease token, worker identity и текст внутренних ошибок в UI-контракт не передаются.

## Fail-closed правила

Read model не должен маскировать противоречивое или неполное состояние. Построение представления завершается ошибкой, если:

- отсутствует обязательная identity/revision информация;
- policy risk не совпадает с risk Change;
- состояние Change противоречит approval evidence;
- исполняемый Change не имеет связанного Job;
- `ChangeID` или action Job не совпадает со связанным Change;
- нарушены границы attempts, version или timestamps;
- терминальное состояние Change противоречит терминальному статусу Job;
- время формирования представления старше authoritative Change/Job evidence.

Если Job уже терминальный, а durable Change ещё не успел отразить результат reconciliation, это не считается успешным завершением. Интерфейс получает `reconciliation_pending=true` и `attention_required=true`.

## Result evidence

Контракт не переносит в основной UI сырые результаты выполнения. Он показывает только наличие и количество подтверждений трёх типов:

- Actual State;
- health evidence;
- audit evidence.

Успешный Job без какого-либо result evidence отображается как требующий внимания. Детальный просмотр evidence должен использовать отдельный авторизованный контракт.

## Timeline

Текущий контракт имеет `timeline.completeness=snapshot_only`.

Это означает, что он отображает только последние authoritative snapshots Change и Job. Исторические переходы не синтезируются из текущего состояния. Полная временная шкала может появиться только после подключения отдельного durable event-history источника.

## Semantic diff

Контракт `ui.operations-semantic-diff/v1` сравнивает две immutable configuration revisions и выдаёт структурный diff:

- `added`;
- `removed`;
- `changed`.

Каждая запись содержит только JSON Pointer path и тип изменения. Значения до/после намеренно не выдаются, потому что configuration revisions могут содержать credentials и другие чувствительные данные.

Ограничения безопасности:

- максимум 256 diff entries;
- максимум 2048 символов в path;
- при превышении лимита diff не обрезается, а отклоняется целиком;
- JSON Pointer segments экранируются по RFC 6901;
- входные revisions должны быть JSON objects.

## Следующие обязательные шаги перед включением в продуктовый UI

1. Подключить read model к authoritative persistence/provider boundary без изменения execution semantics.
2. Зафиксировать RBAC для просмотра Change/Job и semantic diff; не расширять права viewer неявно.
3. Добавить authenticated API/page routing с теми же security headers и no-store политикой, что и другие административные surfaces.
4. Подключить durable event-history только как отдельный источник; до этого сохранять `snapshot_only`.
5. Квалифицировать restart/reconnect, terminal reconciliation, retry/cancel policy, result evidence и recovery-path сценарии.
6. Перед release promotion подтвердить install/upgrade, security и коммерческие gates для точного release tree.
