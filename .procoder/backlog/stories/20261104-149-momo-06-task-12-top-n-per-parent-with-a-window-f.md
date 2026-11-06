# momo-06 Task 12: Top-N per parent with a window function

Status: done 2026-11-06
Created: 2026-11-04
Epic: momo-06-relations
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-06-relations Task 12: Top-N per parent with a window function (`.procoder/plans/momo-06-relations.md:2677`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-33. Files: `load.go`, `load_test.go`.

## Acceptance criteria

- [x] `TestEagerLoadTopNPerParent` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-33] `TestWindowFunctions` asserts ranks per partition; `TestEagerLoadTopNPerParent` asserts at most N children per parent in one query (query-count hook); fails if the strategy falls back to per-parent queries.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Criterion 1: `TestEagerLoadTopNPerParent` (load_topn_test.go) failed first with "LoadPerParentLimit is not supported yet" and passes on Postgres (`go test -tags integration -race`).
- Criterion 2: `TestWindowFunctions` asserts ranks `[1 2 3 1]` per partition; `TestEagerLoadTopNPerParent` asserts exactly one statement via the hook (`len(h.After) == 1`). Mutations killed: m2m partition key, dropped ORDER BY, `<` instead of `<=`, ROW_NUMBER instead of DENSE_RANK for m2m.
- Criterion 3: `procoder check` 0 blocking; make fmt-check vet staticcheck gosec tidy-check clean; `go test -race ./...` green.
