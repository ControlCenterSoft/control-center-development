# Control Center 0.31 — fail-closed promotion gate

Статус: **runner-free prepared slice / НЕ RC / НЕ Public Stable**.

Текущая публичная стабильная база: **0.30.0**. Этот документ относится только к разрешённой линии 0.31.0 и не меняет `VERSION`, не создаёт release/tag и не разрешает публикацию.

## Зачем нужен этот слой

До повышения exact candidate в Release Candidate или Stable должны быть одновременно доказаны техническая, recovery, artifact-integrity и commercial/legal готовность. Отдельный зелёный тест, слово `approved` или наличие checksum сами по себе не являются достаточным evidence.

Promotion gate работает fail-closed: отсутствующее, неизвестное, частичное или некорректно привязанное evidence превращается в blocker.

## Exact revision binding

Для `candidate` и `stable` promotion evidence обязательно привязывается к точным `Version` и immutable commit `Revision`. Значения вида `latest`, имя ветки или иной плавающий указатель не принимаются как release identity.

Привязка проверяется отдельно для qualification/tests, security review, rollback evidence, artifact evidence и commercial/legal evidence. Каждый из этих элементов несёт `ReleaseEvidenceBinding{Version, Revision}`. Evidence от другой версии или другого commit SHA нельзя повторно использовать для текущего candidate: такое смешение fail-closed блокируется как `tests_binding`, `security_binding`, `rollback_binding`, `artifact_binding` или `commercial_binding`.

Это защищает от replay/cross-candidate evidence и не заменяет CI qualification exact SHA; gate только не позволяет представить неполное либо относящееся к другому candidate evidence как готовность к promotion.

## Artifact integrity

Текущий опубликованный Stable 0.30.0 использует проверяемую SHA-256 release identity и provenance. Отдельная detached cryptographic signature в действующей release-модели не заявлена, поэтому 0.31 не должен искусственно выдавать checksum за цифровую подпись или блокировать релиз из-за несуществующего signature-процесса.

Для `candidate` обязательны:

- exact `Version`/`Revision` binding;
- SHA-256 exact binary artifact;
- отдельный checksum sidecar;
- qualification manifest;
- provenance evidence.

Для `stable` дополнительно обязательны:

- SHA-256 exact source artifact;
- `SHA256SUMS`;
- release manifest.

Отсутствие любого обязательного элемента или mismatch binding блокирует promotion.

## Rollback

`candidate` и `stable` требуют `RollbackPrepared=true` и exact `Version`/`Revision` binding rollback evidence. Это соответствует release boundary 0.31: clean-install/upgrade/rollback должны быть доказаны до RC, а не после публикации.

`RollbackPrepared` не означает, что rollback «в принципе возможен». В итоговом release evidence он должен опираться на проверенный для exact candidate путь восстановления с сохранением пользовательских данных, настроек и установленного пользователем пароля `admin`.

## Commercial/legal disposition

Для `candidate` и `stable` требуется bounded `CommercialEvidence`, привязанный к exact `Version`/`Revision`. Он хранит только статус, SHA-256 digest внешнего review evidence и булевы результаты обязательных проверок; legal text, customer data, credentials и иные чувствительные материалы в этот контракт не переносятся.

Обязательные пункты:

- exact `Version`/`Revision` binding;
- итоговый disposition = `approved`;
- валидный `sha256:<64 lowercase hex>` digest review evidence;
- dependency/license review;
- redistribution obligations review;
- THIRD_PARTY_NOTICES readiness;
- source-offer/source-disclosure obligations resolved;
- SBOM prepared;
- применимые EULA/Terms/support/legal requirements dispositioned;
- публичные release/security/HA/SLA claims reviewed и не выходят за подтверждённое test evidence.

Просто установить `Disposition=approved` недостаточно: остальные evidence-пункты и binding проверяются независимо.

## Security boundary

Этот slice:

- не вводит generic shell/command execution;
- не добавляет deployment authority;
- не содержит secrets, tokens, raw Job input/output или персональные данные;
- не меняет migrations;
- не создаёт новых third-party runtime dependencies;
- не объявляет 0.31 RC/Stable;
- не заменяет exact-head Public CI, PostgreSQL/restart qualification, security review или юридическую проверку.

## Development channel

`development` сохраняет прежнюю границу: для обычной разработки обязательны базовые tests/security evidence, но release-only artifact/commercial/rollback evidence не требуется. Это не позволяет трактовать development build как RC/Stable.

## Runner-free проверка slice

Исходный подготовительный work-slice имел локальные side-effect-free проверки `gofmt`, `go test` и `go vet` — PASS. Текущая integration-ветка должна пройти штатную exact-head Public CI перед интеграцией в `main`; этот документ сам по себе не является release evidence.

## Runner policy

Интеграция этого слоя выполняется отдельным release-потоком только при наличии свободной runner-capacity. Дублирующие workflow/rerun не требуются.
