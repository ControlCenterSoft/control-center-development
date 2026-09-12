# Control Center 0.32 — Incident persistence and operator mutations

Status: implementation slice for the 0.32 Incidents / Status / Reports line. It builds on the existing incident domain/query contract and intentionally stays inside the 0.32 release boundary.

## Implemented persistence boundary

This slice adds PostgreSQL persistence for the validated `internal/incidents` read model:

- migration `0013_incident_read_models` with constrained mirrored query columns and JSONB canonical document storage;
- affected-resource index for bounded resource filtering;
- atomic create/replace of the incident document and its resource index;
- optimistic-concurrency replacement using `ObjectPrecondition` plus `ValidateSuccessor` generation/resource-version semantics;
- fail-closed read verification: mirrored columns must exactly match the validated JSON document;
- bounded newest-first list queries with the same keyset pagination and exact filters defined by `incidents.ListQuery`;
- one-extra-row pagination instead of an unbounded count query;
- explicit `MutationWriter` / `MutationRepository` contract for authenticated operator adapters.

## Implemented operator mutation preparation

`internal/incidents/mutation.go` now prepares side-effect-free, optimistic-concurrency-guarded operator mutations without performing authorization or persistence itself:

- **acknowledge**: only `open -> acknowledged`, actor-bound, bounded optional note, timeline event, generation/resource-version successor validation;
- **resolve**: only `acknowledged -> resolved`, mandatory resolution text, optional bounded evidence references, terminal timeline event and rejection of future-dated evidence;
- **runbook/evidence metadata update**: allowed only before resolution, append-only evidence identities, conflicting evidence rebinding rejected, exact evidence repeats ignored, runbook update plus operator timeline note;
- resolved incident metadata is immutable through this mutation path;
- evidence/runbook annotation advances `resource_version` but keeps lifecycle `generation` stable; acknowledge/resolve are semantic lifecycle changes and increment generation;
- every prepared mutation carries the exact `ObjectPrecondition` that persistence must validate atomically.

The mutation helpers clone nested slices/pointers before modification so a failed or successful preparation cannot mutate the caller's current read model in memory.

## Security and integrity rules

Persistence and mutation preparation do **not** authorize callers. HTTP/API adapters must check the dedicated incident operator permission, derive scope from the authenticated principal, enforce any required step-up/MFA policy, and emit immutable audit evidence before exposing a mutation as successful.

The implementation deliberately fails closed when:

- a stored JSON document no longer satisfies the domain contract;
- indexed columns disagree with the JSON document;
- an optimistic concurrency precondition no longer matches;
- a resource version or object ID collides;
- a generation cannot be represented by PostgreSQL `bigint`;
- an operator attempts an invalid lifecycle transition;
- evidence metadata attempts to reuse an existing evidence identity with different digest/time/redaction metadata;
- an evidence reference claims collection after the operator event;
- a no-op metadata request would only churn `resource_version`.

The complete document remains bounded by the domain contract (affected resources, signals, timeline, and evidence counts). Query-critical fields are mirrored only to support indexed filtering; the JSON document remains the authoritative serialized read model.

## Migration and rollback

The up migration creates only new 0.32 tables/indexes and does not mutate existing 0.31 data. The down migration removes child resource indexes/tables before the incident table. No existing Control Center table is dropped or rewritten.

## Follow-up 0.32 integration

Still separate from this slice:

- RBAC/API handler, authenticated scope binding and step-up policy;
- ingestion/correlation write path and immutable audit emission;
- status-page privacy projection;
- email/webhook notification fan-out;
- technical report/API export.

Those layers must consume the incident contracts rather than defining competing structures.

## Test coverage prepared

Runner-free test code now covers acknowledgement successor semantics, stale-precondition rejection, mandatory acknowledgement before resolution, terminal resolution evidence, future-evidence rejection, append-only evidence metadata, stable generation for annotation-only writes, resolved-state immutability, evidence-identity conflict and no-op rejection.

## CI / runner boundary

This work is prepared only on `work/cc032-incidents-persistence-nr1-20260912`. No pull request, workflow dispatch, rerun, check rerun, merge, release action, or other intentional runner-triggering operation is part of this slice. Commits use `[skip ci]`; the repository's push-triggered CI does not target `work/**` branches.
