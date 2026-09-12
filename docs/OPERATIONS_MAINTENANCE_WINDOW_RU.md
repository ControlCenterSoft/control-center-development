# Control Center 0.31 — evidence окна обслуживания для Change

## Назначение

Контракт `ui.operations-maintenance-window/v1` формирует read-only evidence о том, требуется ли для точной immutable revision окна обслуживания и в каком состоянии находится это окно на момент проверки. Он относится к operational workflow Control Center 0.31.0 и предназначен для безопасного отображения условий перед запуском durable Job.

Контракт не является разрешением на выполнение. Даже состояние `open` само по себе не даёт execution authority, не меняет Change state, не заменяет approval, preflight, result verification или recovery policy.

## Привязка к точной revision

Evidence всегда содержит:

- `change_id`;
- `revision_id`;
- `revision_digest` в формате `sha256:<digest>`;
- время `evaluated_at`.

Потребитель обязан сопоставлять эти поля с текущим Change и authoritative immutable revision. Evidence от другой revision или с другим digest нельзя переиспользовать после изменения Change.

## Состояния

- `not_required` — policy не требует окна обслуживания; поле `window` отсутствует;
- `scheduled` — обязательное окно ещё не началось;
- `open` — текущее время находится внутри обязательного окна;
- `expired` — обязательное окно завершилось, и старое evidence нельзя трактовать как разрешение на запуск.

Для обязательного окна задаются `starts_at` и `ends_at`. Длительность ограничена диапазоном от 5 минут до 7 суток. Времена нормализуются в UTC.

## Fail-closed правила

Контракт отклоняется целиком при любом из условий:

- отсутствует или некорректен `change_id` / `revision_id`;
- `revision_digest` не является lowercase SHA-256;
- отсутствует `evaluated_at`;
- обязательное окно не задано;
- окно задано, хотя policy пометила его как необязательное;
- `ends_at` не позже `starts_at`;
- длительность меньше 5 минут или больше 7 суток.

Ошибочный контракт не должен превращаться в `not_required` или `open`.

## Security boundary

`execution_authorized` и `production_mutation_allowed` всегда равны `false`. Контракт не содержит credentials, секретов, произвольных команд, provider-specific access data или host mutation API.

Полный operational admission для Change должен отдельно подтвердить exact-revision approval, актуальный semantic diff, blast radius, preflight, recovery path и остальные policy requirements. Окно обслуживания является только одним из обязательных evidence-компонентов.
