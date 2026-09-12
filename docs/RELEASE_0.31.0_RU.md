# Control Center 0.31.0 — Changes / Jobs operational workflow

Статус документа: **PREPARED / НЕ RELEASE CANDIDATE / НЕ PUBLIC STABLE**.

Текущая публичная стабильная база — Control Center 0.30.0. Этот документ фиксирует release boundary будущей 0.31.0 и не меняет `VERSION`, не создаёт tag/release и не разрешает публикацию.

## Пользовательский результат

0.31.0 формирует основной operational workflow Control Center вокруг типизированных Changes и Jobs. Пользователь должен видеть и различать:

- точную immutable revision Change;
- semantic diff и blast radius;
- execution preflight и maintenance-window evidence;
- approval state/evidence;
- durable Job lifecycle и timeline;
- reconnect/cancel/retry boundaries с exact-version revalidation;
- terminal result и post-condition evidence;
- recovery-path readiness;
- Audit/evidence, позволяющие отличить подтверждённый результат от принятого запроса или неподтверждённого состояния.

Unknown, stale, incomplete или противоречивое evidence не отображается как Healthy/Success.

## Уже интегрированная canonical база

На canonical development `main` после PR #178 присутствуют:

- read-only Changes / Jobs projections;
- revision-bound semantic diff и blast-radius evidence;
- execution preflight и maintenance-window evidence;
- durable Job lifecycle/timeline;
- fail-closed approval/result evidence;
- version-bound cancel/reconnect и production cancellation admission;
- durable Job admission перед enqueue;
- bounded manual-retry admission evidence;
- upgrade-preservation qualification 0.30.0 → candidate 0.31.0;
- bounded recovery-path evidence с exact Change/revision binding и fresh verified backup/restore evidence.

Эта база сама по себе не означает RC/Stable readiness.

## Подготовленные, но ещё не canonical slices

Отдельные runner-free work branches могут содержать следующий код, который нельзя считать частью 0.31 до штатной qualification/integration:

- atomic manual retry lineage/revalidation — отдельный NR1 slice;
- exact-revision operational workflow evidence binding `approval → terminal Job result → recovery`, включая false-success guard, strict storage/transport validator и bounded operator summary — отдельный NR2 slice;
- fail-closed привязку aggregate workflow evidence к текущему Changes / Jobs snapshot с повторной сверкой revision digest, approval state, exact Job version/outcome и result summary — NR2;
- authoritative read-only Changes / Jobs provider поверх текущих Change state machines и durable Job repository, а также runtime API boundary `GET /api/v1/ui/changes-jobs`, защищённый `orchestration.jobs.read` — NR2;
- runner-free E2E test-код, собирающий approval/result/recovery evidence из исходных typed contracts и проводящий его до operator view без подмены failed/blocked semantics — NR2;
- fail-closed Release Candidate readiness aggregator/schema — NR2: привязан к официальному Stable 0.30.0 и его exact Linux artifact digest, принимает только bounded evidence одного candidate SHA и возвращает `ready=true` только после полного набора обязательных gates.

Runtime endpoint намеренно отсутствует, если authoritative provider не подключён; недоступный provider/evidence возвращается fail-closed, а не как пустое успешное состояние. Readiness aggregator не запускает проверки, build/packaging/CI/deploy и не выдаёт publication authority. Наличие этих веток не меняет canonical `main` и не является release evidence.

## Security boundary

0.31.0 не вводит generic shell/command execution API. Risk-bearing mutation остаётся только внутри цепочки:

`Identity/RBAC → Desired State/Change → exact approval → durable Job → typed execution → Actual State/post-condition verification → Audit/evidence → rollback/recovery`.

Обязательные свойства:

- approval привязан к точной revision/hash;
- Job admission повторно проверяет текущую revision/effective approvals непосредственно перед durable binding;
- cancel/retry используют exact durable Job version и fail closed при stale state;
- recovery evidence не выдаёт execution authority;
- terminal `succeeded` без подтверждённого post-condition/health evidence не может стать полным подтверждённым workflow;
- `evidence_complete` не означает `outcome=succeeded`: корректно зафиксированный `failed` outcome остаётся failed;
- Changes / Jobs aggregate доступен только субъекту с глобальным `orchestration.jobs.read`; viewer без этого permission не получает operational Job evidence;
- secrets, credentials, raw Job input/output, lease material и provider-private details не входят в operator evidence contracts.

## Upgrade / data preservation

Кандидат 0.31 должен сохранять при поддерживаемом обновлении с 0.30.0:

- пользовательские данные и настройки;
- установленный пользователем пароль `admin` и first-login/password-change state;
- immutable ранее опубликованные migrations byte-for-byte;
- Change/Job identity, durable state, version и input fingerprint;
- replay/idempotency guarantees;
- rollback/re-apply boundaries.

Любой drift ранее опубликованной migration является release blocker.

## Обязательные оставшиеся gates

До объявления Release Candidate должны быть закрыты на точном итоговом SHA:

1. Qualification/integration atomic manual retry lineage/revalidation.
2. Qualification/integration exact-revision operational workflow evidence binding и authoritative Changes / Jobs runtime API wiring.
3. Полный `approval → Job → verification → recovery` E2E, включая negative/failure/false-success cases; подготовленный runner-free E2E test-код должен пройти exact-head qualification после интеграции.
4. Packaging и clean-install/upgrade/rollback checks для exact candidate artifact.
5. PostgreSQL/restart/reconnect qualification для новых durable boundaries.
6. Финальный security/privacy audit: no-secret boundary, RBAC/scopes, stale/idempotency, Audit integrity и recovery semantics.
7. Финальный commercial/legal disposition: лицензии зависимостей, redistribution/notices/source obligations и отсутствие неподтверждённой commercial-clean claim.
8. Сформировать bounded readiness snapshot для exact candidate SHA; агрегатор обязан оставить любой отсутствующий/pending/blocked gate блокером и не может заменить фактическую qualification.
9. Main qualification после integration и только затем официальный source/tag release.
10. Отдельная Public Stable promotion с binary/source artifacts, SHA-256 sidecar/SHA256SUMS, qualification/release manifests и provenance.

## Release stop conditions

0.31 не выпускается, если остаётся хотя бы одно из следующего:

- false Success или возможность представить непроверенный результат как успешный;
- stale/mismatched revision, approval, Job version или recovery evidence принимается как current;
- rollback/recovery path не доказан для risk-bearing operation;
- migration checksum drift;
- upgrade сбрасывает пользовательский пароль/данные/настройки;
- high-risk security/recovery defect;
- неполная commercial/legal disposition для распространяемых компонентов;
- exact candidate SHA не прошёл обязательную qualification.

## Packaging и публикация

Green check отдельной ветки или PR не является Public Stable. Только точный интегрированный candidate SHA после всех release gates может быть повышен до RC, затем до официального source release и отдельного Public Stable release.
