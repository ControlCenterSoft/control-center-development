# Database migrations

Migrations are ordered and immutable once merged. The current cumulative schema is `0001` through `0006` and covers the resource baseline, local identity/RBAC/audit, persistence invariants, durable Change/Job execution state, first-login password-change enforcement, and CAS-versioned distributed core objects.

Migration `0006` is additive for supported 0.3 installations: it preserves the legacy `organizations`, `resources`, and `config_revisions` tables, adds `cc_core_objects` plus durable idempotency receipts, and creates the single canonical `global` scope when no distributed topology existed before the upgrade.

Apply migrations only to an explicitly selected development/test database. Never place database credentials in this repository.
