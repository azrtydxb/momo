# momo-09 Task 1: QueryEvent pool wait, transaction flag, argument count and dialect

Status: open
Created: 2026-11-04
Epic: momo-09-observability-testing
Sprint: -

## Description

Implements plan momo-09-observability-testing Task 1: QueryEvent pool wait, transaction flag, argument count and dialect (`.procoder/plans/momo-09-observability-testing.md:136`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-18. Files: `db.go`, `internal/obstest/obstest.go`, `obs_helpers_test.go`, `obs_poolwait_test.go`. Contributes to S-18; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestQueryEventPoolWait` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestRecordPoolWaitWithoutHooks` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestQueryEventInTx` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

