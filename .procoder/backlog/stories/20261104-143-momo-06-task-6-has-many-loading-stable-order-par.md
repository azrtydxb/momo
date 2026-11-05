# momo-06 Task 6: Has-many loading, stable order, parent pagination and NULL-key emptiness

Status: open
Created: 2026-11-04
Epic: momo-06-relations
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-06-relations Task 6: Has-many loading, stable order, parent pagination and NULL-key emptiness (`.procoder/plans/momo-06-relations.md:2153`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-6. Files: `load_test.go`, `load.go`.

## Acceptance criteria

- [x] `TestEagerLoadHasMany` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestEagerLoadParentOrderAndPagination` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestEagerLoadNullFKEmpty` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestEagerLoadReloadClearsStaleChildren` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-6, part] this story's tests pass for the part of S-6 this task delivers; the full criterion is verified in story 146.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Red-first: the has-many path was already shipped by Task 5 (`prepareWith`, `finish`, `childQueries` are kind-generic), and the plan states "no logic change expected", so the four plan tests could not fail before this task. Instead each was proven to detect a break by mutation on `load.go`: removing `r.clear` fails `TestEagerLoadReloadClearsStaleChildren`; reversing per-parent child order fails `TestEagerLoadHasMany` and `TestEagerLoadNullFKEmpty`; dropping duplicate-parent grouping fails `TestEagerLoadParentOrderAndPagination`; delivering children to the first owner only fails `TestEagerLoadParentOrderAndPagination`.
- Added `TestEagerLoadHasManyOrderIndependentOfStorage` (an UPDATE moves rows physically; children must still be in primary-key order). Red on the mutant without the primary-key tiebreak in `writeOrder`; the four plan tests alone let that mutant survive.
- Surviving mutants judged equivalent: assigning an empty-but-nil slice (`SetMany(nil)` is the same value), and dropping the key-column ORDER BY (per-parent order is still primary-key order).
- Green: `go test -tags integration -race -count=1 . ./internal/...` against Postgres; `make fmt-check vet staticcheck gosec tidy-check` clean.

