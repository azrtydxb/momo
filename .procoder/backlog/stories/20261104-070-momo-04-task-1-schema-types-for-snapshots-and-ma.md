# momo-04 Task 1: Schema types for snapshots and managed objects

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: -

## Description

Implements plan momo-04-migrations Task 1: Schema types for snapshots and managed objects (`.procoder/plans/momo-04-migrations.md:73`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-1, S-22, S-36, S-37, S-38, S-39. Files: `schema/schema.go`, `schema/types.go`, `schema/format_test.go`.

## Acceptance criteria

- [ ] `TestSchemaFormatVersion` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-1] `TestSchemaJSONRoundTrip` and `FuzzSchemaSort` (schema package): JSON round-trips losslessly and `Sort` makes equivalent schemas byte-identical; fails if a field is dropped from serialization or Sort is order-sensitive.
- [ ] [S-22] `TestReleaseConfig` parses release-please-config.json and asserts the pre-1.0 minor-bump setting and the changelog path; `TestFormatVersions` asserts that a snapshot, checksum file and history table from format N-1 are read by the current version and that a future format version fails with an error naming the required Momo version; fails if a persisted format lacks its version field.
- [ ] [S-36] `TestPostgresSchemas`: models in two non-public schemas migrate, query and round-trip with no diff, and drift reports objects per schema; fails if an identifier is not schema-qualified.
- [ ] [S-37] `TestViewsAsModels`: a view and a materialized view are created by migrations, queried as read-only models, refreshed by the helper; mutation builders on them do not compile; fails if a view is dropped before its dependents.
- [ ] [S-38] `TestRowLevelSecurity`: with a policy applied by migration, a transaction scoped to tenant A cannot read tenant B's rows even through raw SQL, and the setting does not persist on the pooled connection; fails if `set_config` is session-scoped.
- [ ] [S-39] `TestManagedFunctionsTriggersPartitions`: a function, trigger and partitioned table are created by migrations, round-trip with no diff, and a changed function body produces CREATE OR REPLACE; fails if they are reported as unmanaged.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

