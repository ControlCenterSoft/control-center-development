# Distributed Core Contracts 0.4

Этот документ фиксирует реализованный безопасный срез контрактов распределённого Core. Он дополняет `ARCHITECTURE.md`, `ROADMAP.md` и `docs/REQUIREMENTS_RU.md`, но не заменяет их.

## Граница реализации

Пакет `internal/corecontracts` содержит только типы, неизменяемый индекс топологии и чистые функции валидации. Он не записывает данные, не назначает роли на production-узлы, не запускает Jobs и не включает routing/NAT. API-фрагмент `api/openapi-distributed-core.yaml` поэтому не объявляет mutating endpoints.

## Идентичность и версии

Каждый синхронизируемый объект содержит `object_id`, `scope_id`, `owner_scope`, `generation`, `resource_version`, `created_at`, `updated_at`.

- `object_id` — стабильная логическая идентичность;
- `generation` начинается с 1 и увеличивается ровно на 1 только при изменении Desired content;
- `resource_version` — непрозрачная версия persistence/sync слоя, меняется при каждой сохранённой записи и никогда не сравнивается по порядку;
- update/delete обязан атомарно проверить `object_id + resource_version`; опционально проверяется `generation`;
- некорректный precondition соответствует HTTP 400, отсутствие — HTTP 428, несовпадение — HTTP 412.

`ValidateSuccessor` проверяет переход, но не генерирует версию и ничего не сохраняет. Storage adapter обязан выполнять compare-and-swap в одной транзакции с записью.

## Scope, Site и Management Zone

Топология имеет ровно один корневой `global` scope. Вложенные `management`, `site` и `resource` scopes образуют дерево любой глубины; отдельная фиксированная роль Regional Controller не нужна.

Делегирование `rbac`, `policy`, `configuration`, `desired-state` и `applications` всегда явное и действует только для указанного дочернего scope. Пустой список означает deny-by-default, а полномочия родителя не переходят внукам автоматически.

Management Zone — логическая граница политик/RBAC/эксплуатации. Её наличие не включает межсетевую маршрутизацию. Даже роль `edge-gateway` сама по себе является только назначением: forwarding/NAT/port-forwarding появятся лишь через отдельную явную policy на следующем этапе.

## Роли и service identity

Контракт содержит Management Node, Global Controller, Site Controller, Controller Cluster Member, Worker Node, Managed Node, Agent, Data Node, Consensus Node, Repository Node, Telemetry Node, Backup Repository Node и Edge Gateway.

`RoleAssignment.object_id` и `service_identity_id` являются логическими идентичностями, а `target_node_id` указывает физический узел. `ValidateRoleAssignmentSuccessor` проверяет, что при переносе меняются target, Desired generation и `resource_version`, но не логические идентичности. Несколько физических контроллеров одного scope обязаны разделять одну `service_identity_id`; независимые masters одного scope отклоняются. Cluster Coordinator отсутствует в назначаемых ролях, потому что это результат quorum/leader election.

## Desired и Actual State

Desired и Actual State представлены разными типами и не могут быть перезаписаны друг другом.

| Поток | Владелец | Правило |
|---|---|---|
| Desired State | верхний или целевой scope | `owner_scope` равен `scope_id` либо является его предком |
| Actual State | локальный reporting scope | `owner_scope` равен `scope_id` либо является его предком; объект синхронизируется наверх без смены владельца |

Actual State с меньшим `observed_generation` является допустимо отстающим. Значение выше известного Desired generation означает конфликт синхронизации. Состояние считается сходимым только при совпадении generation и статусе `converged`.
