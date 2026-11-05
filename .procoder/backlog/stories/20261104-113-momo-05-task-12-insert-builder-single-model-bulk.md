# momo-05 Task 12: Insert builder: single, model, bulk and conflict handling — S-5

Status: open
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-05-writes-queries Task 12: Insert builder: single, model, bulk and conflict handling — S-5 (`.procoder/plans/momo-05-writes-queries.md:960`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-5. Files: `insert.go`, `insert_test.go`. Contributes to S-5; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestInsertModelOmitAndConflict` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- FAIL before implementation: `go test -run TestInsert .` -> `undefined: momo.Insert`, `undefined: momo.InsertQuery` (build failed).
- PASS after: `go test -tags integration -race -count=1 -run TestInsert .` on PostgreSQL -> ok (TestInsertModelOmitAndConflict/postgres, TestInsertReturningPopulatesIdentity/postgres, plus unit rendering and validation tests).
- Mutation check: removing the FlagIdentity|FlagReadOnly exclusion made TestInsertModelColumnRules, TestInsertBulkAndReturning and TestInsertValidation fail; restored.
- Files: insert.go, insert_test.go, returning.go (ReturningList, appendReturning, ModelRows, needed by Rows).
- Bulk: Models renders one multi-row statement and returns ErrLimitExceeded above dialect.MaxParams; chunking is Task 13.
