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

CI: ci-agent run 42780954032 on headSha bd2ac27668df480d8b40d7d6eaf9db336ec76031, all jobs success. The criteria below are covered by these tests.

- UpdateOf: `TestPostgresInspectTriggerUpdateOf`, `TestPostgresTriggerUpdateOfRoundTrip`, `TestPostgresTriggerUpdateOfKeepsWrittenOrder` and `TestPostgresTriggerUpdateOfOrderRoundTrip` (UPDATE OF b, a keeps its written order, no drift), `TestRenderTriggerPolicyRLS` (UPDATE OF case), `TestGenerateFromDBTriggerUpdateOf` (`--from-db` emits it).
- Exclude before reject: the PostgreSQL inspector applies `Exclude` while reading, skipping a matching table before its columns and constraints, a matching view or function before its definition, and a trigger whose name or table matches before its definition (migrate/postgres/inspect.go, inspect_objects.go). Extensions, enums and policies are still filtered afterwards. `generate --from-db` adds every `introspect.skip` glob to the inspection excludes (cmd/momo/generate_fromdb.go). Tests: `TestPostgresInspectExcludedTableIsNotRead` (a table with a VIRTUAL generated column, which inspection otherwise rejects; skipped when the server is older than PostgreSQL 18), `TestPostgresInspectExcludedViewAndFunction`, `TestPostgresInspectExcludeSkipsTrigger`.
- fk_name: `TestBelongsToForeignKeyName`, `TestForeignKeyNameDiagnostics` (has_many, FKName without belongs_to, over 63 bytes, duplicate name), `TestGenerateFromDBForeignKeyName`, `TestGenerateFromDBForeignKeyNameNotWritable` (only `[A-Za-z_][A-Za-z0-9_$]*` names are written), `TestPostgresForeignKeyNameRoundTrip` (_fkey1 on a partitioned table); diff compares foreign keys by name (a change is a drop and add).
- Primary key order: `Config.PrimaryKey` (no pk:<n> tag); `TestPrimaryKeyOrderFromConfig`, `TestPrimaryKeyOrderMustNameThePKFields`, `TestGenerateFromDBPrimaryKeyOrder`, `TestPostgresPrimaryKeyOrderRoundTrip`.
- Docs: docs/migrations.md, docs/relations.md, docs/ai-guide.md, docs/reference.md.
