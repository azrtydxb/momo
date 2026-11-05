# momo-04 Task 15: Runner, history table, failure recovery, resolve

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: -

## Description

Implements plan momo-04-migrations Task 15: Runner, history table, failure recovery, resolve (`.procoder/plans/momo-04-migrations.md:1720`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-8, S-11, S-22. Files: `migrate/history.go`, `migrate/options.go`. Contributes to S-8, S-11, S-22; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestNonTransactionalFailureRecovery` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestTransactionalFailureRolledBack` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestHistoryFormatVersion` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

