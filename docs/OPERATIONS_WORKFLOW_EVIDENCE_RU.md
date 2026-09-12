# Control Center 0.31 — сквозное evidence operational workflow

## Назначение

Этот слой связывает уже существующие доказательства `approval`, terminal Job result и recovery path в один bounded read-only контракт `ui.operations-workflow-evidence/v1` для **одной точной immutable revision Change**.

Он закрывает отдельную часть сквозного release-gate `approval → Job → verification → recovery`: оператор и последующая qualification могут проверить, что evidence относится к одному `change_id`, одному `revision_id` и одному `revision_digest`, а не к похожим, но различным состояниям.

Контракт не создаёт Job, не запускает worker, не повторяет Job, не выполняет recovery и не разрешает production mutation.

## Exact-revision binding

Слой принимает только уже построенные типизированные evidence:

- `ui.operations-approval-evidence/v1`;
- `ui.operations-job-result-evidence/v1`;
- `ui.operations-recovery-path/v1`.

Для всех трёх должны совпадать:

- `change_id`;
- `revision_id`;
- lowercase SHA-256 `revision_digest`.

Любое расхождение считается противоречивым evidence и отклоняется fail-closed. Также отклоняются future-dated component observations, не-terminal Job result, невалидные digest/identifiers и recovery evidence, пытающееся нести execution authority.

## Привязка к Changes / Jobs operator view

Подготовленный read-only adapter `ApplyVerifiedOperationsWorkflowEvidence` добавляет агрегированное evidence в существующий Changes / Jobs read-model только после повторной сверки с текущим operator snapshot.

Перед публикацией summary он обязан подтвердить:

- `change_id` существует в текущем snapshot;
- `revision_id` совпадает с текущей immutable revision Change;
- `revision_digest` совпадает с authoritative digest revision store;
- approval satisfaction не расходится с текущей approval summary;
- `job_id`, exact durable `job_version` и terminal outcome совпадают с текущим Job projection;
- наличие output evidence, число health/Audit evidence и `worst_health` совпадают с текущим Job result summary;
- aggregate evidence прошло строгую `ValidateOperationsWorkflowEvidence` проверку и не является future-dated.

Если источник aggregate evidence недоступен, UI получает явное `workflow_evidence.availability=unavailable`. Stale/mismatched/duplicate evidence отклоняется полностью; частично обогащённый view не возвращается.

Operator summary содержит только bounded state: availability, `complete|blocked`, exact Job id/version, terminal outcome, ограниченный список blocker reasons и время наблюдения. Raw output/errors, credentials, lease/provider details туда не переносятся.

## Что означает `complete`

`state=complete` означает только, что сквозная цепочка evidence внутренне согласована и содержит обязательные safety evidence. Это **не синоним успешного выполнения**.

Например, завершившийся `failed` Job может иметь `state=complete`, если:

- approval evidence удовлетворено;
- результат Job сохранён и связан с точной revision;
- Audit evidence присутствует;
- recovery path подтверждён и готов.

Таким образом UI и release qualification не превращают корректно зафиксированную ошибку в false Success.

## Когда цепочка `blocked`

Валидное, но неполное safety evidence отражается как `state=blocked` с ограниченным набором причин:

- `approval_not_satisfied` — approval не удовлетворено;
- `recovery_not_ready` — recovery path не готов либо просрочен;
- `post_condition_not_verified` — Job сообщил `succeeded`, но нет health evidence или worst health не `healthy`;
- `audit_evidence_missing` — terminal Job result не содержит Audit evidence.

Противоречивые identity/digest/timestamp данные не переводятся в `blocked`, а полностью отклоняются как невалидный контракт.

## False Success boundary

Для `succeeded` Job контракт требует наличие health checks и `worst_health=healthy`. `degraded`, `failed`, `unknown` или отсутствие health evidence не могут быть представлены как полный подтверждённый workflow.

Для `failed` и `cancelled` outcome контракт сохраняет реальный terminal outcome. `evidence_complete=true` не меняет его и не создаёт формулировку «операция успешна».

## Security / privacy boundary

Сквозной evidence не содержит:

- raw Job input/output;
- `LastError` и provider failure details;
- credentials, secrets, access tokens или lease material;
- mutable policy contents;
- artifact locations/provider endpoints;
- generic shell/command authority.

Поля `execution_authorized` и `production_mutation_allowed` всегда `false`.

## Release boundary

Этот slice относится только к Control Center **0.31.0** и не меняет `VERSION`, SQL schema, ранее опубликованные migrations, runtime dependency graph или коммерческие redistribution obligations.

Runner-free подготовка теперь включает typed aggregate contract, strict validator и fail-closed привязку aggregate evidence к текущему Changes / Jobs operator view. Это всё ещё не делает 0.31 Release Candidate/Public Stable: обязательны exact-head qualification в разрешённом runner-потоке, интеграция подготовленного кода в canonical main, полный operational E2E, install/packaging qualification и финальные ИБ/коммерческие/release gates.
