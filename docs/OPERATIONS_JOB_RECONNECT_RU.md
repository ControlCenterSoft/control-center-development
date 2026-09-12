# Control Center 0.31 — Job reconnect / reload contract

## Назначение

Этот слой закрывает read-side часть `reconnect` для Changes / Jobs operational workflow 0.31.0. После потери соединения, перезагрузки UI или version conflict клиент заново читает `GET /api/v1/jobs/{jobId}` и получает текущее durable-состояние Job вместе с сильным `ETag`, равным точной persisted Job version.

Полученный `ETag` не является разрешением на выполнение. Он только связывает последующее операторское решение с тем состоянием Job, которое было реально прочитано. Для bounded mutation, где требуется optimistic concurrency, клиент передаёт эту точную версию как precondition; stale version не должна автоматически повторяться или заменяться текущей.

## Security boundary

- read остаётся под существующим `orchestration.jobs.read` RBAC;
- ETag добавляется только к успешному авторизованному Job read;
- denied/not-found/error responses не получают durable Job ETag;
- Job response помечается `Cache-Control: no-store`, чтобы operational evidence не сохранялось HTTP-кэшем;
- reconnect не даёт approve/cancel/retry/execution authority и не меняет Job/Change;
- никаких новых секретов, credentials, внешних зависимостей или SQL migrations не добавляется.

## Связь с cancellation

Production cancel path 0.31 требует exact `If-Match` и отклоняет missing/invalid/stale precondition fail-closed. Reconnect/read path предоставляет оператору текущую exact version после reload, но не выполняет cancel автоматически.

## Release boundary

Этот слой не означает Release Candidate или Public Stable сам по себе. После него остаются policy-bounded retry HTTP/admission, recovery path/evidence, end-to-end operational qualification, install/upgrade, packaging, ИБ/коммерческие и остальные release gates по canonical Roadmap.
