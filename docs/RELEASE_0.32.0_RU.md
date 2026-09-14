# Control Center 0.32.0 — Operational Intelligence / Incident Inbox

Status: development

Authoritative Public Stable baseline: **Control Center v0.31.1**.

0.32.0 — текущая линия разработки после Public Stable v0.31.1. Этот документ фиксирует release identity и границу состава версии, но сам по себе не разрешает публикацию тега, GitHub Release или перенос в Public Stable.

## Цель версии

0.32 развивает безопасный operational workflow v0.31.x и добавляет операторский контур Operational Intelligence: Incident Inbox, timeline, responsible-admin workflow и связанные read-only/controlled projections без обхода существующих Identity/RBAC, Change/Approval/Job, verification, Audit и recovery boundaries.

## Release identity

- development identity: `0.32.0`;
- Public Stable baseline: `v0.31.1`;
- следующий официальный тег допускается только для точного qualified SHA;
- `VERSION` определяет линию исходного кода, но **не является разрешением на публикацию**;
- публикация разрешается только отдельным version-specific release gate после exact-SHA qualification.

## Обязательное наследование v0.31.1

0.32.0 обязана сохранять packaging/install hardening, вошедший в v0.31.1, включая полноту deployment-артефакта и обязательное наличие install/migration payload. Нельзя квалифицировать 0.32 относительно более старого v0.31.0 как будто патч v0.31.1 не существовал.

## Текущий статус

Версия находится в разработке. Официальный release и Public Stable **не разрешены** до выполнения 0.32-specific qualification.

Минимальные блокирующие условия перед переводом этого документа в `Status: official release`:

- все обязательные Public CI jobs для точного итогового SHA должны быть PASS;
- должен существовать отдельный 0.32 candidate/package gate, не переиспользующий immutable evidence 0.31;
- clean install, upgrade именно с Public Stable v0.31.1, restart/reconnect и rollback/forward recovery должны быть доказаны;
- PostgreSQL qualification, race tests, security/privacy и public-repository safety должны быть PASS;
- release artifacts, checksums, SBOM/provenance/manifest и их exact-SHA binding должны быть проверены;
- не должно быть false Success, stale/mismatched approval/Job/result/recovery evidence или обхода RBAC/Change/Approval/Job boundaries;
- release notes и version identity должны описывать один и тот же exact release candidate.

## Правило публикации

Пока статус этого документа не равен точному `Status: official release`, release publisher обязан завершаться как явный development/no-publish результат: без создания или перемещения тега, без создания GitHub Release и без promotion в Stable.

Даже после перевода статуса в `official release` публикация 0.32.0 должна fail-closed, если отдельный 0.32 release gate ещё не реализован или его exact-SHA evidence не подтверждён.
