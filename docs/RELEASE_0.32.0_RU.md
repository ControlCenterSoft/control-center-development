# Control Center 0.32.0 — Operational Intelligence / Incident Inbox

Status: official release

Authoritative Public Stable baseline: **Control Center v0.31.1**.

0.32.0 — следующая официально авторизованная линия после Public Stable v0.31.1. Этот документ разрешает выпуск только для точного итогового SHA, который после данного изменения заново прошёл все обязательные проверки и 0.32-specific exact-SHA qualification. Доказательства от более раннего SHA не являются разрешением на публикацию.

## Цель версии

0.32 развивает безопасный operational workflow v0.31.x и добавляет операторский контур Operational Intelligence: Incident Inbox, timeline, responsible-admin workflow и связанные read-only/controlled projections без обхода существующих Identity/RBAC, Change/Approval/Job, verification, Audit и recovery boundaries.

## Release identity

- release identity: `0.32.0`;
- Public Stable baseline: `v0.31.1`;
- официальный тег допускается только для точного qualified SHA;
- `VERSION` определяет линию исходного кода, но **не является самостоятельным разрешением на публикацию**;
- публикация разрешается только version-specific release gate после exact-SHA qualification того же SHA, который содержит этот статус.

## Обязательное наследование v0.31.1

0.32.0 обязана сохранять packaging/install hardening, вошедший в v0.31.1, включая полноту deployment-артефакта и обязательное наличие install/migration payload. Нельзя квалифицировать 0.32 относительно более старого v0.31.0 как будто патч v0.31.1 не существовал.

## Текущий статус

Release authorization для 0.32.0 объявлен, но сама публикация остаётся fail-closed. Итоговый exact SHA после слияния этого изменения обязан заново получить PASS всех обязательных Public CI jobs и отдельного 0.32 exact-SHA qualification gate. Только этот новый PASS может создать неизменяемый source release/tag и открыть последующий перенос в Public Stable.

Обязательные условия выпуска:

- все обязательные Public CI jobs для точного итогового SHA — PASS;
- отдельный 0.32 candidate/package gate — PASS и не переиспользует immutable evidence 0.31;
- clean install, upgrade именно с Public Stable v0.31.1, restart/reconnect и rollback/forward recovery доказаны;
- PostgreSQL qualification, race tests, security/privacy и public-repository safety — PASS;
- release artifacts, checksums, SBOM/provenance/manifest связаны с тем же exact SHA и проверены;
- отсутствуют false Success, stale/mismatched approval/Job/result/recovery evidence и обход RBAC/Change/Approval/Job boundaries;
- release notes, `VERSION`, source tag, qualification evidence и release artifacts описывают один exact release candidate.

## Правило публикации

`Status: official release` является только release authorization. Он не заменяет техническую квалификацию и не разрешает публикацию при отсутствии exact-SHA PASS.

Publisher обязан fail-closed, если запускается от одного Public CI без завершённого 0.32 qualification gate, если evidence относится к другому SHA, если Stable baseline отличается от v0.31.1 или если обнаружен любой release drift. После source release следующий этап — отдельный PR-based promotion в `control-center-stable`; Public Stable считается выпущенным только после успешной проверки уже перенесённого Stable revision.
