# momo-04 Task 21: Staged changes end to end (PostgreSQL)

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

Implements plan momo-04-migrations Task 21: Staged changes end to end (PostgreSQL) (`.procoder/plans/momo-04-migrations.md:2336`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-11. Files: `migrate/staged_test.go`, `migrate/diff.go`.

## Acceptance criteria

- [ ] `TestStagedChange` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestCustomMigrationDriftMismatch` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-11] `TestStagedChange` on a populated DB: add nullable column → data migration backfill → SET NOT NULL applies and all rows satisfy the constraint; fails if diff emits NOT NULL directly for a populated table without a default.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Note (from the Task 8 review): Task 8's `Diff` deliberately returns `ErrStagedRequired` (or, with Stage, a hint and no alter) for a NULL to NOT NULL change without default, and `TestStagedNotNullOnExistingColumn` in `migrate/diff_test.go` asserts it. This task's third diff expects a plain `alter_column` once the snapshot shows the column nullable. The task must therefore change that rule in `migrate/diff.go` and that test together (for example by treating a staged snapshot as the signal that the backfill is done), not just one of them.

### Sprint 7 (2026-11-10)

The shipped design differs from the plan: there is no `-- momo:guard` directive; the follow-up is `momo migrate diff --enforce-not-null` (see bug story 20261106-staged-not-null-is-never-enforced-after-diff-stage). The Task 8 rule from the note above was already changed there.

- [S-11]: `TestStagedChange` (migrate/staged_integration_test.go, PostgreSQL): on a table with three rows, a plain diff refuses with `ErrStagedRequired`; `--stage` adds the column nullable and applies; after a backfill the `--enforce-not-null` migration applies, no row is NULL, `is_nullable` is `NO`, an insert without the column fails, and the next diff is empty.
- `TestCustomMigrationDriftMismatch` (same file): custom SQL adds `docs.extra`; the diff sees no change, drift reports `column docs.extra: not in snapshot`.
- Green: ci-agent run 42776939577 on 0dafa15e799e4e052ca9b05bc94eed3b231d421f, all jobs success.
- Carry reason: the two "exists as written, failed before the implementation" criteria cannot be shown. The behaviour pre-existed (migrate/not_null_staging_test.go, migrate/custom_test.go) and the plan's literal test bodies use an API (`NewCustom(ctx, ...)`, `Diff` without `EnforceNotNull`) that no longer exists, so no red state exists; same decision as story 094.
