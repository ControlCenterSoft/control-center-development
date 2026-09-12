# Control Center 0.32 — Incident read-model persistence

Status: implementation slice for the 0.32 Incidents / Status / Reports line. It builds on the existing `work/cc032-incidents-readmodel-nr2-20260912` domain/query contract and intentionally does not duplicate that work.

## Implemented boundary

This slice adds PostgreSQL persistence for the validated `internal/incidents` read model:

- migration `0013_incident_read_models` with constrained mirrored query columns and JSONB canonical document storage;
- affected-resource index for bounded resource filtering;
- atomic create/replace of the incident document and its resource index;
- optimistic-concurrency replacement using `ObjectPrecondition` plus `ValidateSuccessor` generation/resource-version semantics;
- fail-closed read verification: mirrored columns must exactly match the validated JSON document;
- bounded newest-first list queries with the same keyset pagination and exact filters defined by `incidents.ListQuery`;
- one-extra-row pagination instead of an unbounded count query.

## Security and integrity rules

Persistence does not authorize callers. HTTP/API adapters must check the dedicated incident-read permission before calling this repository and must derive any scope restriction from the authenticated principal rather than trusting arbitrary client scope input.

The repository deliberately fails closed when:

- a stored JSON document no longer satisfies the domain contract;
- indexed columns disagree with the JSON document;
- the optimistic concurrency precondition no longer matches;
- a resource version or object ID collides;
- a generation cannot be represented by PostgreSQL `bigint`.

The complete document remains bounded by the domain contract (affected resources, signals, timeline, and evidence counts). Query-critical fields are mirrored only to support indexed filtering; the JSON document remains the authoritative serialized read model.

## Migration and rollback

The up migration creates only new 0.32 tables/indexes and does not mutate existing 0.31 data. The down migration removes child resource indexes/tables before the incident table. No existing Control Center table is dropped or rewritten.

## Follow-up 0.32 integration

Still separate from this slice:

- RBAC/API handler and authenticated scope binding;
- ingestion/correlation write path and immutable audit emission;
- incident acknowledgement/resolution mutation service;
- status-page privacy projection;
- email/webhook notification fan-out;
- technical report/API export.

Those layers must consume the read-model contract rather than defining competing incident structures.

## CI / runner boundary

This work was prepared on a `work/**` branch only. No pull request, workflow dispatch, rerun, check rerun, merge, release action, or other runner-triggering operation is part of this slice.
