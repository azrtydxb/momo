# momo-06 Task 15: Single-query join loading for to-one relations with NULL-to-nil

Status: open
Created: 2026-11-04
Epic: momo-06-relations
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-06-relations Task 15: Single-query join loading for to-one relations with NULL-to-nil (`.procoder/plans/momo-06-relations.md:3227`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-81. Files: `joinload.go`, `scan.go`, `scan_internal_test.go`, `preload.go`, `select.go`, `preload_test.go`.

## Acceptance criteria

- [x] `TestScanValueNullHandling` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestJoinLoadToOne` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-81] `TestNestedFilteredEagerLoad` and `TestJoinLoadToOne` assert query counts and that a missing to-one leaves nil, not a zero struct; fails if join loading duplicates parents.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Red: before scan.go/joinload.go existed both tests failed to compile (`ScanValue` and `Joined` undefined). Green: `scan_internal_test.go` and `joinload_test.go` pass against PostgreSQL with the integration tag.
- S-81: `TestJoinLoadToOne` asserts one statement, four parents (none duplicated or dropped), a nil Author for the NULL foreign key and no relation event. Also covered: Where/Order/Limit/Offset, One/First, self-reference, composite keys, soft-deleted target kept in ON (SQL shape pinned), several joins, ErrUnsupported cases.
- Mutations, each failing tests: soft-delete in WHERE instead of ON, constant alias (self-reference breaks), no NULL-to-nil detection, composite key joined on the first column only (parents duplicated), DENSE_RANK to ROW_NUMBER for the story-149 follow-up tests.
- Deviations are recorded in the dated reconciliation note after Task 15 in the plan. The discriminator-in-ON case does not arise: belongs-to cannot carry a Discriminator and has-one is not joinable.
