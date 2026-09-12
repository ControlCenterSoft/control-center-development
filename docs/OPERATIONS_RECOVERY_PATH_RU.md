# Control Center 0.31 — recovery path evidence

## Назначение

Контракт `ui.operations-recovery-path/v1` добавляет в operational workflow Control Center 0.31 типизированное read-only evidence о наличии и состоянии пути восстановления для точной immutable revision Change.

Recovery path не является разрешением на выполнение. Даже состояние `ready` само по себе не меняет Change/Job, не выдаёт execution authority, не включает production mutation и не заменяет RBAC, approval, maintenance window, execution preflight, post-condition verification или Audit.

## Привязка к revision

Evidence обязательно содержит:

- `change_id`;
- `revision_id`;
- canonical lowercase `sha256:` digest этой revision;
- `recovery_point_id`;
- наблюдаемое состояние recovery point;
- количество backup records и количество подтверждённых backup records;
- результат restore verification;
- время проверки и, при наличии, срок действия evidence.

При проекции в Changes / Jobs UI запись принимается только если `change_id`, `revision_id` и `revision_digest` совпадают с authoritative состоянием. Неизвестная revision, digest mismatch, future-dated evidence, дубликат или повреждённый контракт приводят к fail-closed ошибке без частично обогащённого view.

## Состояния

`ready` означает только следующее: recovery point находится в `READY`, все заявленные backup records подтверждены, restore verification имеет результат `PASSED`, evidence не истёк.

`blocked` используется, когда recovery point ещё не готов, не все backup records подтверждены или restore verification не прошёл.

`expired` используется, если evidence истёк либо recovery point уже находится в состоянии `EXPIRED`.

Поле `block_reason` является детерминированным и ограничено причинами:

- `recovery_point_not_ready`;
- `backup_not_verified`;
- `restore_not_verified`;
- `evidence_expired`.

## Security boundary

Контракт намеренно не содержит:

- backup artifact path/object key;
- provider endpoint или credentials;
- токены, пароли, ключи и secret material;
- произвольные команды или executable configuration;
- raw provider logs;
- права на restore, retry, cancel или execution.

`execution_authorized` и `production_mutation_allowed` всегда должны быть `false`. Любая запись с `true` отвергается.

## Связь с recovery subsystem

Этот слой не создаёт backup и не выполняет restore. Recovery subsystem остаётся authoritative источником RecoveryPoint/Backup/Restore metadata и restore-drill evidence. Operational evidence — только bounded projection для оператора и последующего release-gate wiring.

Наличие `PASSED` в projection не заменяет проверку provenance recovery metadata и qualification конкретного backup/restore provider. При подключении источника runtime должен получать данные из валидированного recovery metadata graph и не строить `ready` по произвольному клиентскому вводу.

## Связь с release scope 0.31

Срез закрывает typed recovery-path evidence и exact-revision binding для Changes / Jobs operational UI. Он не объявляет 0.31 Release Candidate или Public Stable. До полного release gate отдельно остаются квалификация runtime wiring, install/upgrade, оставшиеся retry/cancel/reconnect сценарии, а также финальные security/commercial checks.
