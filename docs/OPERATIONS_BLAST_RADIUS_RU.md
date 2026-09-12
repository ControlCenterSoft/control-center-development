# Control Center 0.31 — blast radius evidence для Change review

## Назначение

Контракт `ui.operations-blast-radius/v1` формирует ограниченное read-only evidence о ресурсах, которые затрагивает одна точная immutable revision Change. Он относится к закреплённому scope Control Center 0.31.0 и дополняет semantic diff перед approval.

Наличие blast radius evidence само по себе **не разрешает выполнение**. Контракт не переводит Change в `approved`, не создаёт Job, не открывает maintenance window, не разрешает retry/cancel и не заменяет RBAC, policy, preflight, approval или recovery checks.

## Привязка к revision

Evidence обязательно содержит:

- `change_id`;
- точный `revision_id`;
- SHA-256 digest этой revision;
- `observed_at` в UTC.

Потребитель обязан сопоставить `revision_id` и `revision_digest` с authoritative revision store. Evidence от другой или уже заменённой revision нельзя использовать для review текущего Change.

## Состав blast radius

Каждый affected resource содержит только:

- `resource_id` — канонический идентификатор ресурса;
- `kind` — тип ресурса;
- `relation` — `direct` для непосредственной цели либо `dependency` для затронутой зависимости;
- `reason_code` — машинный код причины включения в blast radius.

Свободный текст, конфигурационные значения, provider payload, credentials, токены и значения до/после в контракт не включаются.

Blast radius **не выводится автоматически из semantic diff**. Semantic diff показывает структуру изменения, но не доказывает эксплуатационные зависимости. Список затронутых ресурсов должен поступать от authoritative planner/provider, который действительно знает dependency graph соответствующей capability.

## Fail-closed ограничения

- максимум 256 affected resources;
- превышение лимита приводит к отказу, а не к молчаливому усечению списка;
- `change_id`, `revision_id`, `resource_id` и `kind` должны быть непустыми и каноническими;
- revision digest должен иметь вид `sha256:<64 lowercase hex>`;
- `observed_at` должен быть ненулевым UTC timestamp;
- допустимы только отношения `direct` и `dependency`;
- `reason_code` ограничен безопасным машинным форматом `[a-z0-9._-]` и не предназначен для пользовательского текста;
- один `resource_id + kind` не может встречаться более одного раза;
- ресурсы сериализуются в детерминированном порядке;
- счётчики `resource_count`, `direct_count`, `dependency_count` обязаны точно соответствовать массиву ресурсов.

Любое нарушение делает evidence недействительным целиком. UI не должен заменять invalid/unavailable evidence оптимистическим состоянием «безопасно» или «готово».

## Связь с release scope 0.31

Этот слой закрывает только read-only основу пункта `risk / blast radius` в Changes / Jobs operational UI. Для полного 0.31 по Roadmap отдельно остаются и проверяются approval/preflight semantics, maintenance-window evidence, durable Job lifecycle/timeline, policy-bounded retry/cancel, result evidence и recovery path. Эти элементы нельзя считать реализованными только из-за наличия blast radius контракта.

## Qualification boundary

До интеграции в `main` runner-поток должен проверить exact head как минимум на:

1. детерминированную сортировку;
2. fail-closed отказ при malformed identity/digest/relation/reason code;
3. отказ при duplicate resource identity;
4. отказ при превышении 256 ресурсов;
5. согласованность счётчиков relation/resource count;
6. отсутствие configuration payload и секретов в JSON evidence;
7. сохранение non-authorizing boundary — контракт не должен предоставлять mutation/execution API.
