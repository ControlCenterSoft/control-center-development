# Control Center architecture

Control Center separates request handling, authorization, planning, durable orchestration and execution.

## Request-to-result flow

`Request -> Validate -> Authorize -> Plan -> Change -> Job -> Typed Worker Action -> Verify -> Persist State -> Audit`

## Trust boundaries

- HTTP handlers do not execute arbitrary caller-controlled shell commands.
- RBAC is deny-by-default and enforced server-side.
- Changes bind approvals and policy decisions to explicit operations.
- Jobs are durable and idempotent.
- Worker actions are registered and schema-constrained.
- PostgreSQL persists security- and orchestration-relevant state.

## Runtime

The production-style container runs as a non-root user and is designed for read-only filesystem operation apart from explicitly mounted state.
