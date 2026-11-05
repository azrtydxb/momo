# momo-08 Task 5: migrate/mysql inspection and rendering

Status: open
Created: 2026-11-04
Epic: momo-08-mysql
Sprint: -

## Description

Implements plan momo-08-mysql Task 5: migrate/mysql inspection and rendering (`.procoder/plans/momo-08-mysql.md:680`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-31, S-60. Files: `migrate/mysql/inspect.go`, `migrate/mysql/render.go`, `migrate/mysql/normalize_test.go`, `migrate/mysql/roundtrip_integration_test.go`. Contributes to S-31; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestMySQLNormalizeType` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestMySQLMigrationRoundTrip` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-60, part] `TestMySQLMigrationRoundTrip` passes for the part of S-60 this task delivers; the full criterion is verified in story 204.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

