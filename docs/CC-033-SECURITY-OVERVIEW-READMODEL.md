# Control Center 0.33 — Security Overview read model

Статус: runner-free source slice для последующей интеграции и qualification. Не является доказательством готовности 0.33 и не меняет PUBLIC STABLE.

## Назначение

Контракт `ui.security-overview/v1` формирует безопасное read-only представление для первого экрана будущего milestone 0.33 «Identity / RBAC / Session / Security Settings UI». Он объединяет только уже существующее evidence текущего аутентифицированного пользователя: identity, обязательность смены первоначального пароля, эффективную session policy, активные сессии и effective RBAC grants.

Дополнительный контракт `ui.permission-explanation/v1` формирует bounded объяснение authoritative authorization decision для конкретных permission + scope. Он использует только уже полученные self-only effective grants, проверяет согласованность с server-side `Allowed` и fail closed при decision/grant drift. Это объяснение не является authorization token и всегда возвращает `mutation_authorized=false`.

Оба контракта сознательно **не** добавляют управление пользователями/ролями, не создают новый actor selector и не выдают authorization/execution authority. `self_only=true` и `mutation_authorized=false` являются обязательными границами.

## Security boundary

- Subject задаётся серверным authenticated context, а не пользовательским `subject_id`.
- В projection не попадают password/hash, session token/token digest, credential version, raw repository state или иные секреты.
- Current session должна однозначно присутствовать в active-session evidence; противоречивый marker, expired session или дубликат fail closed.
- Session policy должна иметь положительные bounded TTL/idle semantics; activity не может продлевать absolute expiry.
- RBAC grants принимаются только с валидным scope, canonical role/permission names и без duplicate grant/permission evidence.
- Permission explanation не вычисляет новый доступ: server decision и effective grants обязаны совпадать; иначе ответ не строится.
- Deny explanation различает только безопасные bounded причины: отсутствие grants, отсутствие подходящего scope или отсутствие permission. Внутренние policy/repository details наружу не выдаются.
- Порядок sessions/grants/permissions/matching roles детерминирован, чтобы UI не зависел от порядка backend storage.
- Bounded input: максимум 128 active sessions и 64 effective grants; user-agent ограничен 512 байт.
- `password_change_required` отражается как attention и не обходится read-model слоем.

## Что ещё требуется до интеграции 0.33

1. Подключить builder к authenticated self-only HTTP adapter после объединения 0.32 integration work; не принимать actor/subject из query/body.
2. Получать sessions через существующий self session inventory и grants через `rbac.Introspector.EffectiveGrants` для текущего principal.
3. Сохранять существующую fail-closed Audit boundary: если обязательное security read evidence/Audit недоступно, не отдавать частично «здоровый» security overview.
4. Добавить exact-head HTTP/API tests, schema validation и accessibility/UI rendering tests для overview и permission explanation.
5. Отдельными 0.33 slices реализовать admin-scoped users/roles/bindings UI. Self-only endpoints нельзя расширять параметром выбора другого actor.

## Release boundary

Этот slice не меняет SQL migrations, permissions, runtime dependencies, session semantics или commercial/legal obligations. Он не влияет на текущий 0.31 release candidate path и должен квалифицироваться отдельным runner-потоком только после безопасной интеграции в актуальный 0.33 head.
