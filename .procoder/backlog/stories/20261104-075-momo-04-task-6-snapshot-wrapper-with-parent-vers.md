# momo-04 Task 6: Snapshot wrapper with parent version and format_version

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: -

## Description

Implements plan momo-04-migrations Task 6: Snapshot wrapper with parent version and format_version (`.procoder/plans/momo-04-migrations.md:792`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-11, S-22. Files: `migrate/snapshot.go`. Contributes to S-11, S-22; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestSnapshotParentRecorded` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestSnapshotFormatVersion` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

