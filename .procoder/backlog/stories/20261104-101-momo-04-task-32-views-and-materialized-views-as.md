# momo-04 Task 32: Views and materialized views as read-only models

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: -

## Description

Implements plan momo-04-migrations Task 32: Views and materialized views as read-only models (`.procoder/plans/momo-04-migrations.md:3279`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-37. Files: `internal/compat/viewmodels/viewmodels.go`, `views.go`.

## Acceptance criteria

- [ ] `TestViewInsertDoesNotCompile` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestViewsAsModels` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-37] `TestViewsAsModels`: a view and a materialized view are created by migrations, queried as read-only models, refreshed by the helper; mutation builders on them do not compile; fails if a view is dropped before its dependents.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

