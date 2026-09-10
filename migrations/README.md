# Database migrations

Migrations are ordered and immutable once published. The current cumulative schema is `0001` through `0008` and covers the resource, identity, RBAC, audit, persistence and Change/Job baseline; CAS-versioned distributed core objects; persistent Network Zone/Interface contract types; and additive built-in RBAC synchronization.

Migration `0006` is additive for supported 0.3 installations: it preserves the legacy `organizations`, `resources`, and `config_revisions` tables, adds `cc_core_objects` plus durable idempotency receipts, and creates the single canonical `global` scope when no distributed topology existed before the upgrade.

Migration `0007` extends only the closed `cc_core_objects.object_type` constraint with `network-zone` and `network-interface`. It neither rewrites legacy 0.3 data nor activates network behavior. Its downgrade fails closed while either new object type exists; the operator must explicitly export or remove those objects before reverting.

Migration `0008` synchronizes the persisted permissions and built-in role grants with the built-in Control Center RBAC definitions. It never updates an existing permission definition. A small insertion ledger lets the scoped down migration remove only rows introduced by `0008`, retaining pre-existing custom permissions and grants even when they use a name that later becomes built in.

The cumulative schema requires PostgreSQL 15 or newer because migration `0003` uses `NULLS NOT DISTINCT` uniqueness semantics. Supported migration behavior includes clean schema creation and the documented additive upgrade path from the 0.3 schema line. Before any production upgrade, create and verify a database backup, confirm that the target Control Center release explicitly supports the intended upgrade path, and do not bypass a failed migration or edit migration history manually.

Apply migrations only to the explicitly selected Control Center database. Never place database credentials in product documentation, scripts committed to the repository, or shell history.
