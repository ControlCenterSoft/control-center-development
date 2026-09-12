# Control Center 0.32 — Health Overview read model

Статус: runner-free preparation / source-only / non-production.

## Цель

Этот slice закрывает непокрытую часть milestone 0.32 `Health / Incidents / Audit / Reports`: детерминированное read-only представление Health, которое не показывает устаревшее или просроченное evidence как `Healthy`.

Контракт: `ui.health-overview/v1`.

## Входная граница

`BuildHealthOverview` принимает только уже авторизованные bounded health signals:

- стабильный `signal id`;
- точную пару `resource_kind/resource_id`;
- bounded `check_name`;
- один из существующих health states: `healthy`, `unknown`, `degraded`, `unhealthy`;
- `observed_at`;
- optional bounded `runbook_ref`;
- bounded opaque evidence references.

Raw provider error/message, credentials, worker identity, transport details и произвольный diagnostic payload в контракт намеренно не входят.

## Freshness / false-success policy

Источник обязан задать `stale_after` и более длинный `expired_after` budget.

- `current + healthy` может отображаться как `healthy`;
- `stale + healthy` отображается как `degraded`;
- `expired + healthy` отображается как `unknown`;
- уже наблюдённые `degraded/unhealthy` не улучшаются только из-за старения evidence;
- отсутствие загруженного источника даёт `data_state=unavailable`, `overall_state=unknown`;
- пустой, но успешно загруженный набор сигналов также не объявляется `healthy`.

Таким образом Unknown/Stale/Expired/Unavailable не подменяются зелёным состоянием.

## Агрегация

Read model детерминированно:

- отклоняет duplicate signal IDs и future timestamps;
- нормализует timestamps в UTC;
- deduplicate + sort evidence references;
- агрегирует worst effective state и worst freshness по каждому affected resource;
- сортирует ресурсы и сигналы детерминированно, сначала по риску;
- ограничивает вход до 1000 signals и до 32 evidence refs на signal.

## Security / authority boundary

`mutation_authorized=false` является фиксированным свойством результата. Этот контракт не:

- acknowledge incident;
- создаёт/закрывает incident;
- запускает remediation;
- меняет Desired State;
- выдаёт Change/Job execution authority;
- предоставляет generic shell/command surface;
- добавляет SQL migration или новый runtime dependency.

Будущие acknowledgement/remediation действия должны проходить отдельные RBAC → exact Change/approval → Job → verification → Audit/recovery границы.

## Связь с параллельной 0.32 работой

Slice не дублирует:

- NR1 incident persistence/correlation;
- NR2 incidents read model;
- NR1 safe reporting CSV;
- NR2 bounded Audit export/redaction.

Он создаёт независимую Health-проекцию, которую runner/integration поток сможет позднее связать с Incident/evidence drawer после сведения соответствующих 0.32 branches.

## Qualification handoff

В этой задаче GitHub runner не запускается. Перед интеграцией runner-поток должен:

1. применить/перенести slice на свежий 0.32 integration head;
2. выполнить `gofmt`/`go vet` и focused `go test ./internal/ui`;
3. добавить cross-contract tests с Incident read model после его интеграции;
4. проверить, что stale/expired/unavailable evidence ни в API, ни в UI не становится Healthy;
5. только затем выполнять один штатный exact-head qualification pass без duplicate rerun.
