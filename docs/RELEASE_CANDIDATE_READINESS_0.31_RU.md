# Control Center 0.31 — fail-closed readiness для Release Candidate

## Назначение

Этот слой собирает только bounded evidence о готовности будущего Release Candidate 0.31.0. Он не выполняет проверки сам, не запускает CI/runner, не строит и не публикует артефакты, не меняет `VERSION`, не создаёт tag/Release и не выдаёт deployment/publication authority.

Базовая опубликованная версия зафиксирована точно:

- Stable: `0.30.0`;
- tag: `v0.30.0`;
- официальный Linux amd64 artifact SHA-256: `02d15e8ff13bbcb52b6d0c9293ab8804500991fbb41c8b575e88306a8a5ce0f2`.

Readiness относится к одному точному candidate SHA. Evidence от другого SHA не переиспользуется.

## Обязательные gates

До состояния `ready=true` должны иметь `pass` и bounded SHA-256 evidence все десять gates:

1. `manual_retry_lineage_integration` — mutation-side manual retry lineage/revalidation интегрирован и квалифицирован;
2. `operational_workflow_e2e` — approval → Job → verification → recovery подтверждён без false Success;
3. `candidate_artifact_packaging` — exact candidate artifact собран и его identity/integrity подтверждены;
4. `clean_install` — clean-install exact artifact подтверждён;
5. `upgrade_from_stable_0_30` — поддерживаемый upgrade от официального Stable 0.30.0 подтверждён;
6. `rollback_forward_recovery` — rollback/forward-recovery exact candidate подтверждены;
7. `postgres_restart_reconnect` — durable PostgreSQL/restart/reconnect boundaries подтверждены;
8. `security_privacy` — финальный security/privacy gate подтверждён;
9. `commercial_legal_clearance` — обязательное коммерческое/legal disposition имеет подтверждённый evidence;
10. `release_metadata` — release notes/manifest/provenance/checksums и заявленный scope согласованы с exact candidate.

Отсутствующий, `pending` или `blocked` gate остаётся blocker. Неизвестный или дублированный gate, неправильный Stable artifact, evidence от другого candidate SHA или `pass` без SHA-256 evidence отклоняются как malformed input.

## Граница данных

Readiness snapshot содержит только версии, tag, SHA/digest и статусы gates. В него не должны попадать:

- secrets/credentials/access tokens;
- raw Job input/output;
- логи выполнения;
- персональные данные;
- lease/worker/private provider material;
- внутренние deployment credentials.

Evidence digest является ссылкой на отдельно хранимое проверяемое доказательство, а не заменой самого qualification процесса.

## Fail-closed semantics

`ready=true` вычисляется только при полном наборе известных gates со статусом `pass`, привязанных к одному exact candidate SHA. Любая неполнота возвращает `ready=false` и список blockers. Невалидная идентичность Stable/candidate или неоднозначный набор evidence возвращают ошибку вместо оптимистического результата.

Этот контракт не объявляет 0.31.0 Release Candidate. До штатной exact-head qualification подготовленный runner-free код является только staging work и не считается release evidence.
