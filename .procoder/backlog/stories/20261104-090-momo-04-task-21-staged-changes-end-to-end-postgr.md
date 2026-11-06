# momo-04 Task 21: Staged changes end to end (PostgreSQL)

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: -

## Description

Implements plan momo-04-migrations Task 21: Staged changes end to end (PostgreSQL) (`.procoder/plans/momo-04-migrations.md:2336`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-11. Files: `migrate/staged_test.go`, `migrate/diff.go`.

## Acceptance criteria

- [ ] `TestStagedChange` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestCustomMigrationDriftMismatch` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-11] `TestStagedChange` on a populated DB: add nullable column → data migration backfill → SET NOT NULL applies and all rows satisfy the constraint; fails if diff emits NOT NULL directly for a populated table without a default.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

- Note (from the Task 8 review): Task 8's `Diff` deliberately returns `ErrStagedRequired` (or, with Stage, a hint and no alter) for a NULL to NOT NULL change without default, and `TestStagedNotNullOnExistingColumn` in `migrate/diff_test.go` asserts it. This task's third diff expects a plain `alter_column` once the snapshot shows the column nullable. The task must therefore change that rule in `migrate/diff.go` and that test together (for example by treating a staged snapshot as the signal that the backfill is done), not just one of them.
