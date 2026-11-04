# momo-04 Task 25: PostgreSQL end-to-end apply with the history contract

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: -

## Description

Implements plan momo-04-migrations Task 25: PostgreSQL end-to-end apply with the history contract (`.procoder/plans/momo-04-migrations.md:2699`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-11, S-12. Files: `migrate/postgres/apply_test.go`. Contributes to S-11, S-12; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestCreateIndexConcurrentlyApply` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestPostgresApplyRecovery` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

