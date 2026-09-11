# Control Center 0.27.0 — актуальность проверок восстановления

Статус: подготовка будущей версии; qualification и выпуск выполняются отдельно.

## Назначение

Версия 0.27.0 добавляет детерминированную read-only модель актуальности реально проверенного изолированного восстановления. Она позволяет отличать свежую проверку восстановления от устаревшей и не считать один лишь исторический статус `SUCCEEDED` достаточным доказательством текущей готовности к восстановлению.

## Restore Drill Freshness Assessment

Контракт `recovery.restore-drill-freshness/v1` принимает только валидный `ISOLATED_DRILL` со статусом `SUCCEEDED` и `Verification=PASSED`, связывает оценку с exact `restore_id`, целевым объектом и временем подтверждённой проверки.

Оператор или policy layer задаёт допустимый возраст проверки в целых секундах. Допустимое окно ограничено одним годом. На границе `verified_at + max_age` результат уже считается `STALE`.

`assessment_id` формируется детерминированно из полного freshness claim: restore identity, target, временных границ, policy window, результата и non-authorizing flags. При чтении сохранённого snapshot идентификатор пересчитывается; подмена даже формально корректно выглядящего `assessment_id` отклоняется fail-closed.

## Exact evidence binding

Контракт `recovery.restore-drill-evidence-binding/v1` связывает freshness assessment с точной текущей версией restore metadata и доказательствами проверки:

- `restore_resource_version`;
- `restore_generation`;
- digest канонического набора verification evidence;
- `verified_at`;
- детерминированный `binding_id`.

Повторная проверка binding пересобирает ожидаемое доказательство из текущего restore state. Изменение resource version, generation, verification evidence или assessment делает старый binding `not_current`, а не позволяет повторно использовать устаревшее доказательство.

## Freshness Snapshot

Контракт `recovery.restore-drill-freshness-snapshot/v1` представляет bounded read-only состояние для точного target:

- `FRESH` — последнее валидное успешное restore-drill evidence ещё находится в freshness window;
- `STALE` — доказательство было валидным, но freshness window истёк;
- `UNVERIFIABLE` — для точного target нет доказуемого валидного успешного изолированного restore drill.

При нескольких restore records выбирается самое позднее валидное `verified_at`; равные timestamps разрешаются детерминированно по `restore_id`. Невалидные, failed и non-isolated записи не становятся freshness evidence.

Snapshot хранит exact lineage: restore ID/resource version/generation, assessment ID и digest verification evidence. Добавлены два уровня revalidation:

1. проверка snapshot против одного точного текущего restore record — защищает от resource-version/generation/evidence drift;
2. проверка snapshot против authoritative current restore set — защищает от повторного использования старого self-consistent snapshot после появления более нового успешного restore-drill evidence.

`UNVERIFIABLE` не может быть подтверждён проверкой одного restore record, потому что один объект не доказывает отсутствие других валидных evidence. Он может быть подтверждён только относительно authoritative набора restore records для target.

## Безопасность

Все новые контракты являются только evidence:

- `advisory_only=true`;
- `production_mutation=false`;
- не запускают backup или restore;
- не выполняют fencing;
- не изменяют RecoveryPoint, Desired State или Actual State;
- не предоставляют retry/execution/restore authority;
- устаревшая или неподтверждаемая проверка не исправляется автоматически и требует отдельного управляемого действия согласно политике эксплуатации.

Невалидная restore metadata, непроверенный или неуспешный restore, non-isolated режим, некорректное временное окно, `checked_at` раньше `verified_at`, подмена assessment identity и drift exact evidence отклоняются fail-closed.

## Граница готовности

Наличие этих source contracts и тестового кода не означает готовность 0.27.0 к публикации. Перед promotion версия должна быть перенесена на актуальную qualified baseline, получить новую exact candidate identity и пройти обязательные qualification/release gates отдельным runner-потоком.
