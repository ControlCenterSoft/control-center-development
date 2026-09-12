# Control Center 0.31 — evidence окна обслуживания для Change

## Назначение

Контракт `ui.operations-maintenance-window/v1` формирует read-only evidence о том, требуется ли для точной immutable revision окно обслуживания и в каком состоянии находится это окно на момент проверки. Он относится к operational workflow Control Center 0.31.0 и предназначен для безопасного отображения условий перед запуском durable Job.

Контракт не является разрешением на выполнение. Даже состояние `open` само по себе не даёт execution authority, не меняет Change state, не заменяет approval, preflight, result verification или recovery policy.

## Привязка к точной revision

Evidence всегда содержит:

- `change_id`;
- `revision_id`;
- `revision_digest` в формате `sha256:<digest>`;
- время `evaluated_at`.

Потребитель обязан сопоставлять эти поля с текущим Change и authoritative immutable revision. Evidence от другой revision или с другим digest нельзя переиспользовать после изменения Change.

В представлении «Changes / Jobs» признак доступности окна обслуживания выставляется только после валидации typed-контракта и повторной сверки `change_id`, `revision_id` и `revision_digest` с текущей immutable revision. Если источник evidence не загружен, данные неизвестны, относятся к другой revision, имеют неверный digest, дублируются или датированы будущим временем, интерфейс обязан оставлять maintenance evidence недоступным либо отклонять набор целиком без частично обогащённого результата.

Признак «evidence доступно» означает только наличие проверенного контракта для текущей revision. Он не означает, что окно уже открыто: фактическое состояние определяется отдельным полем `state` и должно оцениваться заново перед admission выполнения.

## Состояния

- `not_required` — policy не требует окна обслуживания; поле `window` отсутствует;
- `scheduled` — обязательное окно ещё не началось;
- `open` — текущее время находится внутри обязательного окна;
- `expired` — обязательное окно завершилось, и старое evidence нельзя трактовать как разрешение на запуск.

Для обязательного окна задаются `starts_at` и `ends_at`. Длительность ограничена диапазоном от 1 минуты до 7 суток — тем же безопасным диапазоном, который использует execution preflight. Времена нормализуются в UTC.

## Fail-closed правила

Контракт отклоняется целиком при любом из условий:

- отсутствует или некорректен `change_id` / `revision_id`;
- `revision_digest` не является lowercase SHA-256;
- отсутствует `evaluated_at`;
- обязательное окно не задано;
- окно задано, хотя policy пометила его как необязательное;
- `ends_at` не позже `starts_at`;
- длительность меньше 1 минуты или больше 7 суток;
- заявленное состояние окна не соответствует `evaluated_at`, `starts_at` и `ends_at`;
- контракт пытается выставить `execution_authorized` или `production_mutation_allowed` в `true`.

Ошибочный контракт не должен превращаться в `not_required`, `open` или доступное evidence.

## Security boundary

`execution_authorized` и `production_mutation_allowed` всегда равны `false`. Контракт не содержит credentials, секретов, произвольных команд, provider-specific access data или host mutation API.

Полный operational admission для Change должен отдельно подтвердить exact-revision approval, актуальный semantic diff, blast radius, execution preflight, recovery path и остальные policy requirements. Окно обслуживания является только одним из обязательных evidence-компонентов.
