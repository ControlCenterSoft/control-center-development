# Control Center 0.31 — безопасное переподключение к durable Job

## Назначение

Контракт `ui.operations-job-reconnect/v1` определяет read-only resume snapshot для повторного подключения интерфейса оператора к уже существующему durable Job после разрыва HTTP/UI-сессии, обновления страницы или временной недоступности клиента.

Reconnect не перезапускает Job, не продлевает lease и не выполняет retry/cancel. Контракт не содержит execution authority и не изменяет серверное состояние.

## Resume cursor

Клиент передаёт `after_version` — последнюю наблюдавшуюся версию Job. Сервер отвечает:

- текущей authoritative версией и состоянием Job;
- `resume_version`, равной текущей версии Job;
- головой persisted lifecycle timeline;
- только lifecycle events с `jobVersion > after_version`;
- признаком `reload_required`, если безопасный bounded delta сформировать нельзя.

Версии Job могут иметь разрывы в timeline. Например, lease renewal изменяет Job version, но намеренно не создаёт отдельное lifecycle event. Поэтому reconnect не требует непрерывной последовательности timeline version; он требует монотонность, точную привязку к одному Job и совпадение головы timeline с текущими lifecycle status/attempt.

## Fail-closed поведение

Полный reload требуется вместо частичного ответа, если:

- клиент прислал cursor новее текущего Job;
- после cursor накопилось более 200 lifecycle events;
- клиентский поток должен быть восстановлен из полного Changes / Jobs snapshot.

Если timeline source недоступен, состояние возвращается как `unavailable`, events не выдаются, `reload_required=true`.

Повреждённый timeline, чужой `job_id`, duplicate/non-monotonic versions, будущие timestamps или несовпадение timeline head с текущим Job считаются ошибкой evidence. В такой ситуации нельзя выдавать частичный delta как достоверный.

## Security boundary

Reconnect snapshot намеренно не содержит:

- Job input/output payload;
- idempotency key;
- lease token или worker identity;
- raw failure/error strings;
- credentials, secrets или provider configuration;
- permission на retry/cancel/execute.

`execution_authorized=false` и `production_mutation_allowed=false` являются обязательными инвариантами.

## Связь с retry/cancel

Reconnect только восстанавливает наблюдение. Наличие статуса `retry_wait`, `running` или `cancel_requested` не означает, что пользователь имеет право выполнить соответствующее действие. Retry/cancel должны иметь отдельный актуальный RBAC/policy check и безопасный repository transition непосредственно в момент команды.

## Граница релиза 0.31

Этот срез закрывает typed reconnect/resume evidence для durable Job и не объявляет 0.31 Release Candidate или Public Stable. Отдельно остаются runtime wiring HTTP/UI, qualification вместе с persisted timeline, policy-gated retry/cancel integration, recovery-path integration, install/upgrade и финальные ИБ/коммерческие gates.
