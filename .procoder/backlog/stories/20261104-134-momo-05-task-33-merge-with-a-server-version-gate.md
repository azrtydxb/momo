# momo-05 Task 33: MERGE with a server-version gate, bulk update from VALUES and Truncate (S-80)

Status: done 2026-11-08
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: -

## Description

Implements plan momo-05-writes-queries Task 33: MERGE with a server-version gate, bulk update from VALUES and Truncate (S-80) (`.procoder/plans/momo-05-writes-queries.md:2724`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-80. Files: `merge.go`, `merge_gate_test.go`, `merge_test.go`, `internal/suite/merge.go`.

Scope rule: the builder must render its statement inside a target-table scope via `Builder.withScope` (builder.go), so a correlated subquery may reference the target table.

## Acceptance criteria

- [x] `TestMergeGate` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestMergeAndBulkUpdate` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-80] `TestMergeAndBulkUpdate` asserts row results on supporting dialects and ErrUnsupported elsewhere; fails if bulk update touches rows outside the VALUES list.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Implemented in merge.go (Merge, BulkUpdate, Truncate, mergeGate) and internal/suite/merge.go; unit tests in merge_gate_test.go and merge_unit_test.go; integration wrapper in merge_test.go.
- Red: run 42455500991 on a throwaway branch with the implementation reverted failed to compile with undefined mergeGate (merge_gate_test.go:24), undefined momo.Merge, momo.BulkUpdate, momo.Truncate (internal/suite/merge.go).
- Green: ci-agent run 42457519128 on 5ee617c686eae450aaaba07859320f13affb3a4f, all jobs success (lint, unit, fuzz-smoke, integration postgres), including TestMergeGate and TestMergeAndBulkUpdate. The PostgreSQL branch asserts the bulk update touches only the listed key (row 1 keeps title and version), MERGE insert and update, RETURNING on PostgreSQL 17, and Truncate; SQLite asserts ErrUnsupported.
