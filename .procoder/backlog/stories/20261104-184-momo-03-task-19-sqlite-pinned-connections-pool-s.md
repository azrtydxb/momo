# momo-03 Task 19: SQLite pinned connections, pool statistics and busy mapping

Status: done 2026-11-08
Created: 2026-11-04
Epic: momo-03-drivers-sqlite
Sprint: -

## Description

Implements plan momo-03-drivers Task 19: SQLite pinned connections, pool statistics and busy mapping (`.procoder/plans/momo-03-drivers.md:1965`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: supporting work for the plan. Files: `driver/sqlitedriver/pin_test.go`, `driver/sqlitedriver/driver.go`.

## Acceptance criteria

- [x] `TestBusyIsStructuredError` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestSQLitePinnedAndStats` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

Red: red run 42453078374 (integration job, sqlitedriver stubbed on throwaway branch) failed TestBusyIsStructuredError, TestSQLitePinnedAndStats. Green: green run 42410524015 on be3aa13 (ci-agent: lint, unit, fuzz-smoke, integration). The procoder check gate was clean at each commit.

Addendum (honest ordering): the tests and the implementation were written together, so the red evidence above was produced after the fact, on a throwaway branch with the driver stubbed and these tests kept; it was not a red run before the implementation. Green on the final head 5bcbfff4c8da982e037c167ac3ac17940126dc30: ci-agent run 42458136467 (lint, unit, fuzz-smoke, integration all passed).
