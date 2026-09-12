# Control Center 0.32 — Health Overview read model

Статус: runner-free source preparation / non-production.

## Цель

Этот slice реализует независимую часть milestone 0.32 `Health / Incidents / Audit / Reports`: детерминированное read-only представление Health, которое не показывает устаревшее, просроченное или недоступное evidence как `Healthy`.

Контракт: `ui.health-overview/v1`.

## Реализовано

- bounded provider-neutral `HealthSignal` без raw provider payload/error, credentials, worker identity и transport details;
- trusted-clock freshness boundary через `stale_after` и `expired_after`;
- `current + healthy -> healthy`;
- `stale + healthy -> degraded`;
- `expired + healthy -> unknown`;
- observed `degraded/unhealthy` не улучшается из-за старения evidence;
- unavailable source и успешно загруженный пустой набор не становятся `healthy`;
- duplicate IDs и future timestamps отклоняются fail-closed;
- evidence refs bounded, trimmed, deduplicated и сортируются;
- deterministic worst-state/worst-freshness aggregation по resource;
- deterministic risk-first sorting;
- `mutation_authorized=false` фиксирован в модели и JSON Schema;
- focused negative/freshness/aggregation test code подготовлен без запуска GitHub runner.

## Ограничения

Slice не:

- создаёт, acknowledge или закрывает Incident;
- запускает remediation;
- меняет Desired State;
- выдаёт Change/Job execution authority;
- добавляет generic shell/command surface;
- добавляет SQL migration или новый runtime dependency;
- меняет Stable/RC identity.

Любые будущие acknowledgement/remediation действия должны проходить отдельную цепочку RBAC → exact Change/approval → Job → verification → Audit/recovery.

## Не дублирует параллельную работу

На момент подготовки slice отдельно существуют NR1 ветки по incident persistence/correlation/operator HTTP boundary и safe reporting, а также NR2 ветки по bounded Audit export/redaction и раннему health contract draft. Этот branch перенесён на фактический current `main` после merge PR #183 и добавляет именно source implementation + contract + tests Health projection.

## Runner handoff

Эта ветка намеренно не имеет PR: `pull_request` запускает Public CI, что запрещено для текущего non-runner прохода. `Public CI` на push ограничен `main`, `release/**`, `develop/**`, `migration/**`; source сохраняется только в `work/**`.

Перед интеграцией runner-поток должен:

1. сверить branch с актуальным 0.32 integration head;
2. выполнить `gofmt`/`go vet` и focused `go test ./internal/ui`;
3. проверить JSON Schema contract;
4. добавить cross-contract tests с Incident/evidence drawer после интеграции соответствующих 0.32 slices;
5. проверить, что stale/expired/unavailable evidence ни в API, ни в UI не становится Healthy;
6. затем выполнить один штатный exact-head qualification pass без duplicate rerun.
