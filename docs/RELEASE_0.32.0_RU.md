# Control Center 0.32.0 — Health / Incidents / Audit / Reports

Статус: **КАНДИДАТ НА PUBLIC STABLE; НЕ ОПУБЛИКОВАН**.

Эта страница описывает целевой состав 0.32.0. Версия становится официальным PUBLIC STABLE только после успешной проверки одного exact SHA, формирования неизменяемого комплекта артефактов и отдельной публикации `v0.32.0`.

## Пользовательский результат

Control Center 0.32.0 объединяет эксплуатационную диагностику и доказательства в безопасном read-only контуре:

- Health Overview с явной свежестью и состояниями unavailable/stale/degraded;
- Reports / Evidence Drawer для ограниченного просмотра подтверждающих данных;
- отдельные authenticated read-only представления Audit и Incidents;
- ограниченный CSV-export Audit;
- source-specific server-side RBAC без наследования более слабого разрешения;
- понятное разделение принятого запроса, выполняемого Job и подтверждённого результата.

Unknown, stale, incomplete или противоречивое evidence не отображается как Healthy/Success.

## Граница безопасности

0.32.0 не добавляет универсальный command/shell API и не предоставляет автоматическую remediation, provider execution, external publication или коммерческую authority.

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

До публикации exact 0.32.0 candidate обязан подтвердить:

- clean install;
- поддерживаемое обновление с текущего Public Stable 0.31.1;
- сохранность данных, настроек, пользовательского пароля `admin`, first-login state и durable Change/Job/timeline state;
- PostgreSQL 15/16/17/18 migration и adapter paths;
- неизменность migrations 0001–0012 и корректность additive migration `0013_incident_read_models`;
- restart/race qualification;
- rollback к сохранённому pre-upgrade state и последующий forward recovery;
- post-condition verification без false Success.

## Обязательный release-комплект

Официальная публикация должна связать с одним квалифицированным exact SHA:

- `control-center-0.32.0-linux-amd64.tar.gz`;
- SHA-256 sidecar и `SHA256SUMS`;
- source archive;
- qualification manifest;
- release manifest;
- provenance;
- CycloneDX SBOM;
- `THIRD_PARTY_NOTICES.md`.

Тег, артефакты, контрольные суммы и manifests после публикации не переписываются.

## Коммерческий статус

Техническая готовность PUBLIC STABLE и commercial launch — разные решения. Незавершённая коммерческая легализация не удерживает технически готовый релиз, если коммерческий runtime выключен и продуктовые security, upgrade, rollback и recovery gates пройдены. При этом отсутствие legal/commercial evidence не должно обозначаться как commercial PASS.

## Критерий публикации

`v0.32.0` разрешён только после успешных build/test/package/install/upgrade/rollback/recovery/security gates для одного нового exact SHA. Source, PR, RC или локальный PASS сами по себе не являются Stable.
