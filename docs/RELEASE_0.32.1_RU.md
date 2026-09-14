# Control Center 0.32.1 — Operational Intelligence / Incident Inbox

Status: official release

Authoritative Public Stable baseline: **Control Center v0.31.1**.

0.32.1 — corrective full-scope source identity after immutable source release v0.32.0. Published v0.32.0 tag/assets are not changed or reused as evidence for this candidate. Qualification, artifacts and any subsequent Stable promotion must bind only to the new exact 0.32.1 SHA.

0.32.1 — текущая линия разработки после Public Stable v0.31.1. Этот документ фиксирует release identity и границу состава версии, но сам по себе не разрешает публикацию тега, GitHub Release или перенос в Public Stable.

## Цель версии

0.32 развивает безопасный operational workflow v0.31.x и добавляет операторский контур Operational Intelligence: Incident Inbox, timeline, responsible-admin workflow и связанные read-only/controlled projections без обхода существующих Identity/RBAC, Change/Approval/Job, verification, Audit и recovery boundaries.

## Пользовательский результат

Целевой состав 0.32.1 объединяет эксплуатационную диагностику и доказательства в безопасном read-only контуре:

- Health Overview с явной свежестью и состояниями unavailable/stale/degraded;
- Reports / Evidence Drawer для ограниченного просмотра подтверждающих данных;
- отдельные authenticated read-only представления Audit и Incidents;
- ограниченный CSV-export Audit;
- source-specific server-side RBAC без наследования более слабого разрешения;
- понятное разделение принятого запроса, выполняемого Job и подтверждённого результата.

Unknown, stale, incomplete или противоречивое evidence не отображается как Healthy/Success.

## Release identity

- development identity: `0.32.1`;
- Public Stable baseline: `v0.31.1`;
- следующий официальный тег допускается только для точного qualified SHA;
- `VERSION` определяет линию исходного кода, но **не является разрешением на публикацию**;
- публикация разрешается только отдельным version-specific release gate после exact-SHA qualification.

## Обязательное наследование v0.31.1

0.32.1 обязана сохранять packaging/install hardening, вошедший в v0.31.1, включая полноту deployment-артефакта и обязательное наличие install/migration payload. Нельзя квалифицировать 0.32 относительно более старого v0.31.0 как будто патч v0.31.1 не существовал.

## Граница безопасности

0.32.1 не добавляет универсальный command/shell API и не предоставляет автоматическую remediation, provider execution, external publication или коммерческую authority.

Обязательные свойства:

- actor определяется серверной session identity;
- Incident, Health, Reports и Audit защищены собственными permission checks;
- raw secrets, credentials, SourceIP и неограниченные Details не выдаются в операторский UI;
- read-only UI не содержит mutation controls и не рекламирует mutation endpoints;
- ответы с чувствительными operational данными используют `no-store` и необходимые security headers;
- published migrations предыдущих версий остаются immutable byte-for-byte;
- first-login `admin/admin` требует обязательной смены пароля, а обновление не сбрасывает установленный пароль;
- commercial capabilities остаются default-off до отдельного legal/security/privacy допуска.

## Установка, обновление и восстановление

До публикации exact 0.32.1 candidate обязан подтвердить:

- clean install;
- поддерживаемое обновление с текущего Public Stable 0.31.1;
- сохранность данных, настроек, пользовательского пароля `admin`, first-login state и durable Change/Job/timeline state;
- PostgreSQL 15/16/17/18 migration и adapter paths;
- неизменность опубликованных migrations и корректность additive migration `0013_incident_read_models`;
- restart/race qualification;
- rollback к сохранённому pre-upgrade state и последующий forward recovery;
- post-condition verification без false Success.

## Обязательный release-комплект

Официальная публикация должна связать с одним квалифицированным exact SHA:

- `control-center-0.32.1-linux-amd64.tar.gz`;
- SHA-256 sidecar и `SHA256SUMS`;
- source archive;
- qualification manifest;
- release manifest;
- provenance;
- CycloneDX SBOM;
- `THIRD_PARTY_NOTICES.md`.

Тег, артефакты, контрольные суммы и manifests после публикации не переписываются.

## Текущий статус

Release authorization для corrective 0.32.1 объявлен, но публикация остаётся fail-closed до нового exact-SHA Public CI и отдельной 0.32 qualification. Public Stable не считается выпущенным до отдельного verified promotion в Stable-репозитории.

Обязательные блокирующие условия перед публикацией:

- все обязательные Public CI jobs для точного итогового SHA должны быть PASS;
- должен существовать отдельный 0.32 candidate/package gate, не переиспользующий immutable evidence 0.31;
- clean install, upgrade именно с Public Stable v0.31.1, restart/reconnect и rollback/forward recovery должны быть доказаны;
- PostgreSQL qualification, race tests, security/privacy и public-repository safety должны быть PASS;
- release artifacts, checksums, SBOM/provenance/manifest и их exact-SHA binding должны быть проверены;
- не должно быть false Success, stale/mismatched approval/Job/result/recovery evidence или обхода RBAC/Change/Approval/Job boundaries;
- release notes и version identity должны описывать один и тот же exact release candidate.

## Коммерческий статус

Техническая готовность PUBLIC STABLE и commercial launch — разные решения. Незавершённая коммерческая легализация не удерживает технически готовый релиз, если коммерческий runtime выключен и продуктовые security, upgrade, rollback и recovery gates пройдены. При этом отсутствие legal/commercial evidence не обозначается как commercial PASS.

## Правило публикации

`Status: official release` является только release authorization и не заменяет технические gates. Publisher обязан fail-closed, если exact candidate, Public CI, qualification bundle, checksums, provenance, manifest или Stable baseline не совпадают. Существующий v0.32.0 source release остаётся immutable и не может считаться evidence для 0.32.1.
