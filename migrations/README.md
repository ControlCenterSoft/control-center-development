# Database migrations

Migrations are ordered and immutable once merged. The current cumulative schema is `0001` through `0005` and covers the resource baseline, local identity/RBAC/audit, persistence invariants, durable Change/Job execution state, and first-login password-change enforcement.

Apply migrations only to an explicitly selected development/test database. Never place database credentials in this repository.
