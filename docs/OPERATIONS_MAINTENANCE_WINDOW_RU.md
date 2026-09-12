# Control Center 0.31 — maintenance window evidence для Change review

## Назначение

Контракт `ui.operations-maintenance-window/v1` даёт интерфейсу read-only представление окна обслуживания, связанного с одной точной immutable revision Change. Это часть закреплённого scope 0.31.0 для Changes / Jobs operational UI.

Наличие окна **не является разрешением на выполнение**. Контракт не заменяет RBAC, policy decision, approvals, preflight, semantic diff, blast radius, Job admission, post-condition verification или recovery. Даже состояние `active` не означает, что Job можно запускать автоматически.

## Привязка

Evidence содержит:

- `window_id`;
- `change_id`;
- точный `revision_id` и SHA-256 digest;
- `starts_at` и `ends_at`;
- время получения authoritative evidence `observed_at`;
- явное время вычисления состояния `evaluated_at`;
- состояние `scheduled`, `active` или `expired`.

Все временные поля нормализуются в UTC. `ends_at` обязан быть строго позже `starts_at`. Future-dated `observed_at` отклоняется fail-closed.

## Семантика состояния

Состояние вычисляется только относительно явного `evaluated_at`:

- `scheduled` — `evaluated_at < starts_at`;
- `active` — `starts_at <= evaluated_at < ends_at`;
- `expired` — `evaluated_at >= ends_at`.

Persisted/provider evidence считается недействительным, если сохранённое `state` не соответствует этим границам. Это исключает оптимистическое отображение старого окна как активного.

## Fail-closed правила

- identity поля не могут быть пустыми или иметь внешние пробелы;
- максимальная длина identity — 255 символов;
- revision digest — только `sha256:<64 lowercase hex>`;
- все timestamp должны быть ненулевыми UTC timestamps;
- `observed_at` не может быть позже `evaluated_at`;
- недопустим нулевой или обратный временной диапазон;
- typed binder в Changes / Jobs принимает evidence только для существующего Change, совпадающего `revision_id` и authoritative revision digest;
- future-dated `observed_at/evaluated_at` относительно текущего read snapshot отклоняются;
- при unloaded evidence source интерфейс обязан показывать `maintenance_window=unavailable`, а не сохранять ранее оптимистическое значение.

## Граница 0.31

Этот слой закрывает read-only основу maintenance-window evidence. Он намеренно не добавляет API создания/изменения окна и не предоставляет execution authority. Mutation path должен оставаться общим: Identity/RBAC → exact Change revision → policy/approval/preflight → durable Job → verification → Audit/evidence → recovery.

## Qualification boundary

Перед интеграцией runner-поток должен квалифицировать exact head минимум на:

1. scheduled/active/expired boundary cases;
2. malformed identity/digest/timestamps;
3. future-dated evidence;
4. reverse/zero-length window;
5. exact revision/digest binding;
6. fail-closed поведение при unloaded source;
7. отсутствие approve/queue/execute/retry/cancel authority в контракте и binder.
