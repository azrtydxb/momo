# momo-04 Task 12: PostgreSQL inspection: views, functions, triggers, policies, RLS, partitions

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: -

## Description

Implements plan momo-04-migrations Task 12: PostgreSQL inspection: views, functions, triggers, policies, RLS, partitions (`.procoder/plans/momo-04-migrations.md:1320`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-37, S-38, S-39. Files: `migrate/postgres/inspect_objects.go`.

## Acceptance criteria

- [ ] `TestPostgresObjectInspection` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-37] `TestViewsAsModels`: a view and a materialized view are created by migrations, queried as read-only models, refreshed by the helper; mutation builders on them do not compile; fails if a view is dropped before its dependents.
- [ ] [S-38] `TestRowLevelSecurity`: with a policy applied by migration, a transaction scoped to tenant A cannot read tenant B's rows even through raw SQL, and the setting does not persist on the pooled connection; fails if `set_config` is session-scoped.
- [ ] [S-39] `TestManagedFunctionsTriggersPartitions`: a function, trigger and partitioned table are created by migrations, round-trip with no diff, and a changed function body produces CREATE OR REPLACE; fails if they are reported as unmanaged.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

