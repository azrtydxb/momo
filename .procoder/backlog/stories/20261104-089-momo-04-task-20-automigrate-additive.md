# momo-04 Task 20: AutoMigrate additive

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: -

## Description

Implements plan momo-04-migrations Task 20: AutoMigrate additive (`.procoder/plans/momo-04-migrations.md:2272`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-11, S-14. Files: `migrate/automigrate.go`. Contributes to S-11; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestAutoMigrateAdditive` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-14] `TestAutoMigrateAdditive`: AutoMigrate creates missing tables/columns/indexes, returns skipped changes for a type change and a removed column, and the column still exists afterwards; fails if AutoMigrate ever emits DROP or ALTER TYPE.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

