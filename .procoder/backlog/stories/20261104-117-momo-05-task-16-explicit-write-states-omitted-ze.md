# momo-05 Task 16: Explicit write states: omitted, zero, NULL and DEFAULT (S-5)

Status: open
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-05-writes-queries Task 16: Explicit write states: omitted, zero, NULL and DEFAULT (S-5) (`.procoder/plans/momo-05-writes-queries.md:1223`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-5. Files: `write_states_test.go`, `internal/suite/writes.go`, `insert.go`, `update.go`. Contributes to S-5; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestWriteStates` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestWriteStatesInsert` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestWriteStatesMultiRow` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

