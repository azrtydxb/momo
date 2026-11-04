# momo-04 Task 27: Rebase onto a target directory or version

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: -

## Description

Implements plan momo-04-migrations Task 27: Rebase onto a target directory or version (`.procoder/plans/momo-04-migrations.md:2827`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-27. Files: `migrate/rebase.go`.

## Acceptance criteria

- [ ] `TestMigrateRebase` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-27] `TestMigrationBranchConflict`: two migrations with the same parent snapshot fail `momo migrate validate`; `TestMigrateRebase` regenerates the later one on top of the other and the result applies with a no-change round trip; fails if two heads are accepted.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

