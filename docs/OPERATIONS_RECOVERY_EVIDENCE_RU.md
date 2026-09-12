# Control Center 0.31 — recovery evidence для Change review

## Назначение

Контракт `ui.operations-recovery-evidence/v1` даёт интерфейсу read-only доказательство того, какой recovery path подготовлен для одной точной immutable revision Change. Это часть закреплённого scope 0.31.0 для Changes / Jobs operational UI.

Наличие recovery evidence не является разрешением на выполнение Change, Job или самого recovery. Оно не заменяет RBAC, policy, approvals, maintenance window, preflight, result verification и Audit.

## Привязка

Evidence содержит:

- `recovery_plan_id`;
- `change_id`;
- точный `revision_id` и SHA-256 digest;
- стратегию `rollback`, `rollforward` или `manual_recovery`;
- readiness `ready`, `degraded` или `unavailable`;
- `observed_at` и `validated_at`;
- SHA-256 digest verification evidence;
- явный признак `operator_action_required`.

Все timestamps нормализуются в UTC. Evidence с будущим `observed_at` или `validated_at` отклоняется fail-closed на границе read-model.

## Fail-closed правила

- identity поля обязательны, canonical и ограничены 255 символами;
- revision и verification digests имеют вид `sha256:<64 lowercase hex>`;
- recovery evidence принимается только для существующего Change и его текущей точной revision;
- digest revision обязан совпадать с authoritative digest этой revision;
- больше одного recovery evidence для одного Change в одном snapshot запрещено;
- `manual_recovery` всегда требует действия оператора;
- `degraded` и `unavailable` всегда требуют действия оператора;
- при unloaded evidence source UI обязан показывать `recovery_evidence=unavailable`, а не сохранять ранее известное состояние;
- наличие `ready` не означает автоматического запуска recovery и не является заменой approval/policy.

## Граница 0.31

Этот слой закрывает безопасную read-only основу Recovery для Change review. Он не создаёт mutation API и не выполняет rollback/rollforward. Исполняемая часть остаётся в общем lifecycle: Identity/RBAC → exact Change revision → review/policy/approval/preflight → durable Job → post-condition verification → Audit/evidence → recovery.

## Qualification boundary

Перед интеграцией runner-поток должен квалифицировать exact head минимум на:

1. exact revision/digest binding;
2. malformed identity/digest/timestamps;
3. future-dated evidence;
4. duplicate evidence для одного Change;
5. `manual_recovery` без operator action;
6. degraded/unavailable без operator action;
7. fail-closed поведение при unloaded source;
8. отсутствие скрытого mutation/recovery authority в contract и binder.
