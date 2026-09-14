# Control Center 0.32.0 — qualification gate

Статус: release qualification contract.

Этот документ не является разрешением на публикацию и не меняет `Status: development` в `docs/RELEASE_0.32.0_RU.md`.

## Цепочка допуска

`Public CI PASS на exact main SHA` → `Qualify Control Center 0.32 exact SHA PASS` → `qualified source release v0.32.0` → отдельное продвижение в `control-center-stable` → Stable verification → Public Stable.

## Обязательные свойства gate

- кандидат — только точный 40-символьный SHA текущего `main`;
- `VERSION` обязан быть `0.32.0`;
- исходный Public CI должен завершиться `success` на том же SHA;
- пакет Linux amd64 собирается дважды из одного SHA и `SOURCE_DATE_EPOCH` и обязан быть воспроизводимым;
- SBOM и license inventory сверяются с текущим `go.mod` и checked-in license evidence;
- чистая установка проверяется на пакете кандидата;
- поддерживаемое обновление выполняется от неизменяемого Public Stable `v0.31.1` с SHA-256 `b9d6467c7c95a6e7e8597398c1b6e7327d319058d248e9cd0416c5baf9699c97`;
- rollback/recovery проверяется восстановлением pre-upgrade snapshot и повторным forward migration;
- qualification/provenance/release-manifest привязаны к exact candidate SHA;
- qualification evidence не предоставляет publication authority самостоятельно.

## Fail-closed публикация

Для `0.32.0` общий publisher не принимает обычный `Public CI` как достаточное основание. Он принимает только успешное завершение workflow `Qualify Control Center 0.32 exact SHA`, скачивает artifact с именем, содержащим exact SHA, проверяет `SHA256SUMS`, qualification/provenance/release-manifest и повторно подтверждает исходный Public CI run на том же SHA.

Пока `docs/RELEASE_0.32.0_RU.md` имеет `Status: development`, tag и GitHub Release не создаются даже при успешной qualification.

Qualified source release в development-репозитории не считается Public Stable. Public Stable считается завершённым только после отдельного продвижения того же доказанного source SHA в `control-center-stable`, прохождения Stable verification и публикации Stable release из Stable-контура.
