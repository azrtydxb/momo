# momo-04 Task 14: PostgreSQL migration lock and session timeouts

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-04-migrations Task 14: PostgreSQL migration lock and session timeouts (`.procoder/plans/momo-04-migrations.md:1620`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-11, S-28. Files: `migrate/postgres/lock.go`. Contributes to S-11; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestLockHolderNamed` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestLockKeyPerSchema` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestLockTimeoutApplied` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-28, part] `TestLockTimeoutApplied` passes for the part of S-28 this task delivers; the full criterion is verified in story 095.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

