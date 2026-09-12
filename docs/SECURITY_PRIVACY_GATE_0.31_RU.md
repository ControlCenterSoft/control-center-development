# Control Center 0.31 — финальный security/privacy gate

## Назначение

Этот контракт фиксирует bounded evidence для обязательного `security_privacy` gate Control Center 0.31.0. Он относится к одному точному candidate SHA и предназначен для fail-closed включения фактического security/privacy disposition в общий Release Candidate readiness snapshot.

Сам контракт **не выполняет** security-проверки, не запускает CI/runner, не сканирует инфраструктуру, не собирает артефакты, не изменяет release channel и не выдаёт production authority. Наличие заполненного JSON без подтверждённого внешнего evidence не является PASS.

## Обязательная привязка

Evidence содержит:

- `candidate_version=0.31.0`;
- точный 40-символьный `candidate_sha`;
- disposition `pending`, `blocked` или `approved`;
- SHA-256 digest внешнего audit/review evidence;
- полный набор security/privacy review dimensions;
- детерминированный `snapshot_digest`, рассчитанный из всех bounded полей.

Изменение candidate SHA, disposition, внешнего evidence digest или любого review-флага меняет `snapshot_digest`. Evidence от другого candidate не переиспользуется.

## Что должно быть подтверждено до PASS

`approved` является необходимым, но недостаточным условием. Все следующие признаки должны быть `true`:

1. `no_secret_boundary_reviewed` — release/readiness/operational evidence не раскрывает credentials, tokens, raw secrets, private provider material или другие секреты;
2. `rbac_scopes_reviewed` — существующие Identity/RBAC/scopes не ослаблены, denied path остаётся fail-closed;
3. `stale_replay_idempotency_reviewed` — stale revision/version, replay и uncertain-response paths не создают скрытую повторную mutation или false Success;
4. `audit_integrity_reviewed` — административные и security-sensitive действия сохраняют требуемую Audit/evidence traceability и не обходят integrity boundary;
5. `recovery_semantics_reviewed` — failure/recovery/rollback semantics не объявляют неподтверждённое восстановление или успех;
6. `data_minimization_reviewed` — API/UI/release evidence содержит только необходимые поля и не расширяет сбор/выдачу данных без необходимости;
7. `error_evidence_redaction_reviewed` — errors/evidence не экспортируют raw Job input/output, credentials, provider payloads или чувствительные внутренние детали;
8. `production_authority_unchanged` — release slice не создаёт generic shell, не расширяет production mutation authority и не обходит Change/Job/approval/recovery boundaries;
9. `critical_security_findings_closed` — нет открытых критических security findings для exact candidate;
10. `critical_privacy_findings_closed` — нет открытых критических privacy findings для exact candidate.

Если хотя бы один признак отсутствует, итоговый gate становится `blocked`. Метка `approved` без полного evidence набора не может дать PASS.

## Целостность snapshot

`SecurityPrivacySnapshotDigest` детерминированно нормализует schema/version/SHA/disposition/evidence digest и все review-флаги, сериализует bounded payload и вычисляет SHA-256. Валидатор повторно вычисляет digest и отклоняет tampered snapshot.

`audit_evidence_digest` и `snapshot_digest` являются ссылками/идентификаторами доказательств, а не местом хранения самих отчётов. В этот контракт запрещено помещать логи, секреты, персональные данные, raw Job payloads или внутренние deployment credentials.

## Связь с Release Candidate readiness

После успешной фактической проверки валидатор формирует только:

- `gate=security_privacy`;
- `status=pass` либо `blocked`;
- exact `candidate_sha`;
- bounded `evidence_digest` = `snapshot_digest`.

Общий `release-candidate-readiness.v1` по-прежнему требует все остальные независимые gates: operational workflow, packaging, install/upgrade, rollback/recovery, PostgreSQL/reconnect, commercial/legal и release metadata. Security/privacy PASS сам по себе не объявляет 0.31.0 Release Candidate или Public Stable.

## Runner-free граница текущей подготовки

Этот source slice подготовлен как runner-free работа. Он может быть квалифицирован и интегрирован только отдельным штатным runner/release потоком. До такой exact-head qualification его нельзя использовать как фактическое доказательство PASS или основание для promotion.
