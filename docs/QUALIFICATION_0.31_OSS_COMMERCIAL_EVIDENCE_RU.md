# Control Center 0.31 — OSS / SBOM / commercial evidence

Статус: **engineering evidence only / НЕ commercial clearance / НЕ RC / НЕ Public Stable**.

## Что закрывает этот slice

Для exact candidate 0.31 packaging теперь формируются и проверяются:

- точный dependency/license inventory из `third_party/manifest-0.31.json`;
- проверка полного соответствия inventory текущему `go.mod`;
- SHA-256 каждого включённого текста лицензии;
- CycloneDX 1.7 SBOM, привязанный к exact candidate SHA;
- `THIRD_PARTY_NOTICES.md` с точными версиями и SPDX;
- включение SBOM, notices и license evidence в distributable candidate;
- SHA-256 linkage этих evidence-artifacts в provenance/release manifest/SHA256SUMS;
- fail-closed проверка расширенного candidate artifact set.

Текущий Go dependency graph содержит только MIT и BSD-3-Clause компоненты, относящиеся к ALLOW-классу инженерной OSS policy. Это не является самостоятельным юридическим заключением.

## Qualification boundary

После любого изменения кода, форматирования или набора сохраняемых candidate artifacts требуется ровно один hosted Public CI на текущем exact PR head. Результаты предыдущего head не переиспользуются как qualification нового candidate identity; duplicate workflow, synthetic load и бессмысленные rerun запрещены.

## Что этот slice намеренно НЕ закрывает

`commercial_legal_clearance` остаётся закрытым. Техническая генерация SBOM/notices не заменяет:

- утверждение юридического лица/лицензиара и применимого права;
- финальные EULA/Terms/Privacy/Support/SLA;
- решение по рынкам/юрисдикциям и B2B/B2C;
- финальную модель лицензирования, pricing/billing/refund/support;
- квалифицированный legal review применимых обязательств;
- финальное vulnerability/security disposition exact candidate.

До получения этих evidence promotion gate должен оставаться fail-closed.

## Release boundary

Наличие `SBOM=PASS` и `THIRD_PARTY_NOTICES=PASS` означает только инженерную готовность соответствующих subgates. Оно не разрешает публикацию 0.31 и не изменяет canonical Stable 0.30.0.
