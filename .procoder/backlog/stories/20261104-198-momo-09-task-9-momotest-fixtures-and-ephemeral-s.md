# momo-09 Task 9: momotest fixtures and ephemeral SQLite

Status: open
Created: 2026-11-04
Epic: momo-09-observability-testing-sqlite
Sprint: -

## Description

Implements plan momo-09-observability-testing Task 9: momotest fixtures and ephemeral SQLite (`.procoder/plans/momo-09-observability-testing.md:1520`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-34. Files: `momotest/fixtures.go`, `momotest/sqlite.go`, `momotest/testdata/fixtures.yaml`, `momotest/fixtures_test.go`, `go.mod`.

## Acceptance criteria

- [ ] `TestMomotestFixtures` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestMomotestSQLiteEphemeral` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-34] `TestMomotestTxRollback` asserts data written in a momotest transaction is absent after cleanup; `TestMomotestSQLAssert` asserts the helper fails on SQL mismatch; fails if cleanup commits.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

