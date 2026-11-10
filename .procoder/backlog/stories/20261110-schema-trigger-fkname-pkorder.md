# Schema: UPDATE OF triggers, exclude before reject, fk_name, primary-key order (issues #58, #59, #60)

Status: open
Created: 2026-11-10
Epic: momo-04-migrations-core-postgresql
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

The Kuvryn Owl port's `--from-db` and `baseline` rehearsal is blocked or noisy in three ways:

- #58: inspection fails outright on `AFTER UPDATE OF state ON submission`. Listing the trigger in `migrate.exclude`/`introspect.skip` doesn't help, because the error is raised before the filter runs.
- #59: a foreign key named `_fkey1`, which PostgreSQL picks on partitioned tables, cannot be declared, so baseline reports it.
- #60: a composite primary key is always declared in field order, but `PRIMARY KEY (name, scope, at)` on `(at, scope, name, value)` matters for btree order.

## Acceptance criteria

- [ ] `schema.Trigger` models an `UPDATE OF` column list: rendered in DDL, inspected back, emitted by `--from-db`, stable under diff (PostgreSQL integration test with the issue's trigger)
- [ ] exclude/skip filters run before any object is rejected as unrepresentable, so an excluded unsupported object no longer fails inspection (test)
- [ ] `fk_name:<name>` on `belongs_to` (and the Config form) sets the constraint name; diff and baseline compare it; `--from-db` writes it when it differs from the derived name (integration test with a `_fkey1` on a partitioned table)
- [ ] the primary-key order can be declared independently of field order (`c.PrimaryKey(...)` or a `pk:<n>` position), inspected back the same, emitted by `--from-db`, and baseline on `health_sample` reports no change (integration test); docs updated
