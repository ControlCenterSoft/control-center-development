# Control Center 0.32 — qualification обновления с Public Stable 0.31.1

Статус: release-gate evidence для линии 0.32. Этот документ не объявляет 0.32 Release Candidate или Public Stable.

## Цель

Зафиксировать точную границу обновления от текущего опубликованного Public Stable Control Center 0.31.1 к 0.32 schema line. Public Stable 0.31.1 использует тот же immutable schema boundary, что и 0.31.0, и включает migrations `0001`–`0012`; первая новая migration линии 0.32 — `0013_incident_read_models`.

Qualification обязана доказать nearest-Stable путь 0.31.1 → 0.32. Patch 0.31.1 не добавляет SQL migration поверх 0.31.0, но его exact release identity и сохранённое состояние должны быть явной исходной границей; добавление incident persistence не переписывает ранее опубликованное состояние и не ослабляет существующие Identity/RBAC/Job/Audit/recovery границы.

## Проверяемый сценарий

`TestPostgresUpgradeFrom0311PreservesIdentityAndJobState` выполняется на одноразовой PostgreSQL-базе и проверяет:

1. установку точного Public Stable 0.31.1 migration boundary `0001`–`0012`;
2. создание локального `admin`, смену bootstrap-пароля и сохранение `password_change_required=false`;
3. существующие immutable revision, Change, durable Job и Job timeline до обновления;
4. применение `0013_incident_read_models.up.sql`;
5. появление только 0.32 incident tables без изменения существующего 0.31.1 состояния;
6. идемпотентный replay `0013`;
7. scoped rollback только `0013`, при котором 0.31.1 состояние остаётся доступным и неизменным;
8. повторное применение `0013` и PostgreSQL restart с повторной проверкой состояния.

## Release boundary

PASS этого теста закрывает только migration/upgrade-preservation часть 0.32. Для Public Stable 0.32 отдельно остаются обязательными полный Roadmap scope, exact-head qualification, security/privacy, packaging, install/upgrade/rollback, release metadata/provenance/checksums и version-specific Stable promotion gate.

Commercial/legal документы ведутся отдельным commercial-launch track и не преобразуются в фиктивный технический PASS.
