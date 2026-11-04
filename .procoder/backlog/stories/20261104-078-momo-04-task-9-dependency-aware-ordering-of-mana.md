# momo-04 Task 9: Dependency-aware ordering of managed objects

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: -

## Description

Implements plan momo-04-migrations Task 9: Dependency-aware ordering of managed objects (`.procoder/plans/momo-04-migrations.md:1063`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-36, S-37, S-39. Files: `migrate/order.go`.

## Acceptance criteria

- [ ] `TestDropViewBeforeDependentView` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestCreateViewAfterDependency` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestCycleIsAnError` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-36] `TestPostgresSchemas`: models in two non-public schemas migrate, query and round-trip with no diff, and drift reports objects per schema; fails if an identifier is not schema-qualified.
- [ ] [S-37] `TestViewsAsModels`: a view and a materialized view are created by migrations, queried as read-only models, refreshed by the helper; mutation builders on them do not compile; fails if a view is dropped before its dependents.
- [ ] [S-39] `TestManagedFunctionsTriggersPartitions`: a function, trigger and partitioned table are created by migrations, round-trip with no diff, and a changed function body produces CREATE OR REPLACE; fails if they are reported as unmanaged.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

