# Control Center

Control Center is a centralized infrastructure-management platform built around a typed, auditable execution model.

Current development baseline: **0.3.0**.

## Core capabilities in the current baseline

- HTTP/JSON API and health/readiness endpoints;
- local identity and session management;
- deny-by-default RBAC;
- append-oriented audit chain;
- PostgreSQL-backed durable state;
- immutable configuration revisions;
- policy/risk/approval-aware Changes;
- durable Jobs with leases, retries and idempotency;
- allowlisted typed Worker actions;
- resource state and health model;
- containerized non-root runtime.

## Development model

Development proceeds in parallel branches. Every branch is validated by public GitHub-hosted CI using standard runners. Pull requests are expected to pass format, vet, race tests, unit/integration tests and build gates before merge.

No credentials, private infrastructure topology, production data or private deployment endpoints belong in this repository.

## Local checks

```bash
make ci
```

## Local build

```bash
make build
./bin/control-center
```
