# Control Center 0.28.0 — Network Verification Freshness

Статус: **release candidate**. Базовая версия Public Stable — **0.27.0**. Публикация допускается только после успешной qualification точного итогового SHA и последующих canonical release/Public Stable gates.

## Основное изменение

Control Center 0.28.0 усиливает безопасный контур сетевых изменений: результат проверки связности рассматривается как ограниченное по времени доказательство, привязанное к точному каноническому плану изменения и его ревизии.

Проверка не является разрешением на изменение сети и не выполняет сетевые команды. Она подтверждает только то, что обязательные проверки для конкретного `ChangePlan` полны, успешны и остаются актуальными непосредственно перед переходом к следующему этапу безопасной state machine.

## Что добавлено

- привязка verification evidence к точным `plan_id` и `revision_id`;
- обязательное свежее доказательство для каждого `ConnectivityProbe.ID`, объявленного каноническим планом;
- повторная проверка целостности всего переданного `ChangePlan`, включая envelope-поля, staged steps и default-deny семантику;
- детерминированные SHA-256 identity для verification evidence и preflight admission;
- контроль максимального возраста evidence и ограниченного clock skew;
- fail-closed обработка отсутствующих, устаревших и завершившихся `FAIL` проверок;
- строгая правая граница срока действия: момент `observed_at + max_age` / `expires_at` уже считается истёкшим;
- preflight admission, связанный с точной версией state machine;
- строгий consumer boundary для admission: неизвестные поля, trailing JSON, дубликаты и неканонический порядок rejection reasons отклоняются;
- защита от повторного использования admission после изменения плана, ревизии, evidence, состояния либо версии state machine;
- повторная проверка authoritative verification evidence непосредственно на границе перехода из `preflight`: `evidence_id` обязан совпадать с фактическим evidence, freshness пересчитывается на момент перехода, а `expires_at` admission обязан точно соответствовать сроку жизни исходного evidence. Самосогласованный SHA-256 admission не считается доказательством происхождения и не может самостоятельно открыть `apply_window`.

## Информационная безопасность

- `ready=true` не предоставляет права на выполнение сетевых изменений;
- `execution_authorized=false` и `production_mutation_allowed=false` остаются обязательными;
- сетевое изменение продолжает проходить отдельные этапы authorization, staged apply, connectivity verification и rollback;
- старое evidence нельзя использовать для нового или изменённого сетевого плана;
- сериализованный admission является снимком результата, а не доверенным источником истины: перед переходом state machine он повторно связывается с authoritative evidence;
- self-consistent admission с подменённым `evidence_id` либо искусственно продлённым `expires_at` отклоняется fail-closed;
- изменение WAN/LAN-конфигурации само по себе не включает routing, NAT или port-forwarding;
- неизвестное, повреждённое, неоднозначное или устаревшее состояние не трактуется как успешное;
- scope 0.28.0 не добавляет новый сетевой listener, credential flow, хранение секретов или внешний runtime dependency.

## Коммерческая и лицензионная граница

Изменения 0.28.0 не добавляют сторонних runtime-компонентов и не меняют dependency graph продукта: scope состоит из собственных Go-контрактов/валидации, JSON Schema, тестов и русскоязычной документации. Новых обязательств по redistribution, source-offer или NOTICE из-за этого scope не возникает.

Существующие требования Market к license/SPDX expression, authoritative source, distribution mode, commercial/redistribution disposition и versioned evidence сохраняются без ослабления. Публичная публикация должна пройти штатные license/compliance и artifact/provenance gates; текст release notes не заменяет машинно проверяемые release evidence.

## Совместимость, установка и обновление

Изменение является аддитивным на уровне сетевых safety-контрактов и не требует новой SQL migration либо разрушительного изменения существующих данных. Опубликованные ранее миграции не должны изменяться.

Обновление не должно автоматически применять сетевую конфигурацию: фактическая мутация сети остаётся отдельной явно разрешённой операцией. Scope 0.28.0 не меняет пользовательские credentials и сам по себе не выполняет сетевых изменений.

Внутренний вызов перехода `preflight` теперь требует передать то же authoritative verification evidence и freshness policy, на основании которых был построен admission. Это намеренное усиление fail-closed границы до публикации 0.28.0; внешний сетевой runtime/API не расширяется.

Перед выпуском точный итоговый SHA обязан пройти штатные clean-install и supported-upgrade проверки на поддерживаемых PostgreSQL, adapter/restart qualification, race/static/unit/build и public-source safety gates.

## Проверки для qualification

Перед выпуском должны быть подтверждены как минимум следующие сценарии:

- корректное свежее evidence для точного плана;
- несовпадение `plan_id` или `revision_id`;
- изменение полей плана при сохранении старого `plan_id`;
- отсутствующий, `FAIL` или устаревший обязательный probe;
- timestamps из будущего вне разрешённого clock skew;
- точная граница истечения freshness/admission;
- подмена evidence/admission identity;
- самосогласованный admission с подменённым `evidence_id`;
- искусственное продление `expires_at` admission после истечения authoritative evidence;
- изменение версии либо состояния preflight state machine;
- неизвестные поля, trailing JSON, дубли и неканонические rejection reasons.

## Packaging и публикация

Наличие исходного кода, release notes или зелёных проверок предыдущего SHA не является основанием для публикации. Для выпуска требуются зелёные проверки точного итогового SHA, merge в канонический release path, официальный tag/release и отдельная Public Stable promotion с предусмотренными binary/source artifacts, SHA-256 checksums, qualification/release manifests и provenance.

Дополнительные внешние review-сигналы не заменяют deterministic CI, security qualification, branch protection или обязательные release gates и не являются единственным основанием для блокировки выпуска при недоступности внешнего провайдера.

## Граница готовности

Версия считается готовой к promotion только после qualification **точного итогового SHA 0.28.0**. Нельзя переносить PASS от предыдущего SHA после изменения кода, release identity или metadata. До завершения этого цикла 0.28.0 остаётся release candidate и не должна описываться как Public Stable.
