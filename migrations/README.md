# Database migrations

Migrations are ordered and immutable once merged. The current cumulative schema is `0001` through `0004` and covers the resource baseline, local identity/RBAC/audit, persistence invariants, and durable Change/Job execution state.

Apply migrations only to an explicitly selected development/test database. Never place database credentials in this repository.
