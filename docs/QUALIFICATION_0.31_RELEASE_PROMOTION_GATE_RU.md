# Control Center 0.31 — fail-closed promotion gate

Статус: **runner-free prepared slice / НЕ RC / НЕ Public Stable**.

Текущая публичная стабильная база: **0.30.0**. Этот документ относится только к разрешённой линии 0.31.0 и не меняет `VERSION`, не создаёт release/tag и не разрешает публикацию.

## Зачем нужен этот слой

До повышения exact candidate в Release Candidate или Stable должны быть одновременно доказаны техническая, recovery, artifact-integrity и commercial/legal готовность. Отдельный зелёный тест, слово `approved` или наличие checksum сами по себе не являются достаточным evidence.

Promotion gate работает fail-closed: отсутствующее, неизвестное, частичное или некорректно привязанное evidence превращается в blocker.

## Exact revision binding

Для `candidate` и `stable` promotion evidence обязательно привязывается к точному commit revision. Значения вида `latest`, имя ветки или иной плавающий указатель не принимаются как release identity.

Это не заменяет CI qualification exact SHA; gate только не позволяет представить неполное evidence как готовность к promotion.

## Artifact integrity

Текущий опубликованный Stable 0.30.0 использует проверяемую SHA-256 release identity и provenance. Отдельная detached cryptographic signature в действующей инструкции не заявлена, поэтому 0.31 не должен искусственно выдавать checksum за цифровую подпись или блокировать релиз из-за несуществующего signature-процесса.

Для `candidate` обязательны:

- SHA-256 exact binary artifact;
- отдельный checksum sidecar;
- qualification manifest;
- provenance evidence.

Для `stable` дополнительно обязательны:

- SHA-256 exact source artifact;
- `SHA256SUMS`;
- release manifest.

Отсутствие любого обязательного элемента блокирует promotion.

## Rollback

`candidate` и `stable` требуют `RollbackPrepared=true`. Это соответствует release boundary 0.31: clean-install/upgrade/rollback должны быть доказаны до RC, а не после публикации.

`RollbackPrepared` не означает, что rollback «в принципе возможен». В итоговом release evidence он должен опираться на проверенный для exact candidate путь восстановления с сохранением пользовательских данных, настроек и установленного пользователем пароля `admin`.

## Commercial/legal disposition

Для `candidate` и `stable` требуется bounded `CommercialEvidence`. Он хранит только статус, SHA-256 digest внешнего review evidence и булевы результаты обязательных проверок; legal text, customer data, credentials и иные чувствительные материалы в этот контракт не переносятся.

Обязательные пункты:

- итоговый disposition = `approved`;
- валидный `sha256:<64 lowercase hex>` digest review evidence;
- dependency/license review;
- redistribution obligations review;
- THIRD_PARTY_NOTICES readiness;
- source-offer/source-disclosure obligations resolved;
- SBOM prepared;
- применимые EULA/Terms/support/legal requirements dispositioned;
- публичные release/security/HA/SLA claims reviewed и не выходят за подтверждённое test evidence.

Просто установить `Disposition=approved` недостаточно: остальные evidence-пункты проверяются независимо.

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

## Runner-free проверка этого slice

При подготовке ветки выполнены только локальные side-effect-free проверки выбранного `internal/release` slice:

- `gofmt` — без diff;
- `go test` для изолированного release-gate package — PASS;
- `go vet` для изолированного release-gate package — PASS.

GitHub-hosted runner не использовался. Полная repository qualification должна выполняться отдельным runner/release-потоком после интеграции work branch в его exact-head candidate.
