# Schema: UPDATE OF triggers, exclude before reject, fk_name, primary-key order (issues #58, #59, #60)

Status: done
Created: 2026-11-10
Epic: momo-04-migrations-core-postgresql
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

The Kuvryn Owl port's `--from-db` and `baseline` rehearsal is blocked or noisy in three ways:

- #58: inspection fails outright on `AFTER UPDATE OF state ON submission`. Listing the trigger in `migrate.exclude`/`introspect.skip` doesn't help, because the error is raised before the filter runs.
- #59: a foreign key named `_fkey1`, which PostgreSQL picks on partitioned tables, cannot be declared, so baseline reports it.
- #60: a composite primary key is always declared in field order, but `PRIMARY KEY (name, scope, at)` on `(at, scope, name, value)` matters for btree order.

## Acceptance criteria

- [x] `schema.Trigger` models an `UPDATE OF` column list: rendered in DDL, inspected back, emitted by `--from-db`, stable under diff (PostgreSQL integration test with the issue's trigger)
- [x] exclude/skip filters run before any object is rejected as unrepresentable, so an excluded unsupported object no longer fails inspection (test)
- [x] `fk_name:<name>` on `belongs_to` (and the Config form) sets the constraint name; diff and baseline compare it; `--from-db` writes it when it differs from the derived name (integration test with a `_fkey1` on a partitioned table)
- [x] the primary-key order can be declared independently of field order (`c.PrimaryKey(...)` or a `pk:<n>` position), inspected back the same, emitted by `--from-db`, and baseline on `health_sample` reports no change (integration test); docs updated

## Evidence

CI: ci-agent run 42774470850 on headSha eed34e25a92ab40fa6dad97bf1c1f97cbec133ff, all 9 jobs success.

- UpdateOf: `TestPostgresInspectTriggerUpdateOf`, `TestPostgresTriggerUpdateOfRoundTrip` (migrate/postgres), `TestRenderTriggerPolicyRLS` (UPDATE OF case). `--from-db` emits the field through the reflection-based Trigger emit.
- Exclude before reject: `TestPostgresInspectExcludeSkipsTrigger`; the PostgreSQL inspector skips an excluded trigger or table before reading it (migrate/postgres/inspect_objects.go).
- fk_name: `TestBelongsToForeignKeyName` (gen), `TestGenerateFromDBForeignKeyName` (gen/fromdb), `TestPostgresForeignKeyNameRoundTrip` (_fkey1 on a partitioned table), tag grammar test; diff compares foreign keys by name.
- Primary key order: `Config.PrimaryKey` (no pk:<n> tag); `TestPrimaryKeyOrderFromConfig`, `TestPrimaryKeyOrderMustNameThePKFields` (gen), `TestGenerateFromDBPrimaryKeyOrder` (gen/fromdb), `TestPostgresPrimaryKeyOrderRoundTrip`.
- Docs: docs/migrations.md, docs/relations.md, docs/ai-guide.md, docs/reference.md.
