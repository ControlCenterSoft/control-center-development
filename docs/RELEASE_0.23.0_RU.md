# Control Center 0.23.0 — RBAC self-introspection

Статус: официальный source release.

## Что нового

Control Center 0.23.0 добавляет безопасный read-only API для просмотра текущим аутентифицированным пользователем собственных назначений RBAC.

- `GET /api/v1/identity/self/access` возвращает только права текущей identity; выбрать другого пользователя через запрос невозможно.
- Ответ содержит детерминированно упорядоченные роли, scope и permissions, включая явный wildcard `*` без искусственного разворачивания.
- Пользователь без bindings получает пустой список grants без неявного разрешения доступа.
- API недоступен до обязательной смены первоначального пароля после чистой установки.
- Чтение security-sensitive данных фиксируется в Audit; при недоступности обязательного Audit endpoint работает fail-closed.
- Ответ помечается `Cache-Control: no-store` и не содержит пароли, session tokens, cookie или token digests.
- Реализация поддерживает одинаковую модель introspection для in-memory authorizer и PostgreSQL-backed RBAC.

## Проверка релиза

Опубликованная release identity должна соответствовать точному квалифицированному исходному дереву. Обязательные release gates проверяют unit/integration behavior, format/vet/build, public-safety, PostgreSQL clean-install/supported-upgrade и adapter compatibility. Публикация версии допускается только после успешного прохождения обязательных проверок и не предоставляет дополнительных runtime-полномочий сама по себе.
