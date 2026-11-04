# momo-04 Task 19: Optional authored down migrations and group rollback

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: -

## Description

Implements plan momo-04-migrations Task 19: Optional authored down migrations and group rollback (`.procoder/plans/momo-04-migrations.md:2194`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-11, S-94. Files: `migrate/rollback.go`. Contributes to S-11, S-94; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestRollbackGroup` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestRollbackNoDown` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

