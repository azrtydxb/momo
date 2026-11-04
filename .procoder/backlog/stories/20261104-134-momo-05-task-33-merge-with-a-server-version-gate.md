# momo-05 Task 33: MERGE with a server-version gate, bulk update from VALUES and Truncate (S-80)

Status: open
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: -

## Description

Implements plan momo-05-writes-queries Task 33: MERGE with a server-version gate, bulk update from VALUES and Truncate (S-80) (`.procoder/plans/momo-05-writes-queries.md:2724`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-80. Files: `merge.go`, `merge_gate_test.go`, `merge_test.go`, `internal/suite/merge.go`.

## Acceptance criteria

- [ ] `TestMergeGate` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestMergeAndBulkUpdate` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-80] `TestMergeAndBulkUpdate` asserts row results on supporting dialects and ErrUnsupported elsewhere; fails if bulk update touches rows outside the VALUES list.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

