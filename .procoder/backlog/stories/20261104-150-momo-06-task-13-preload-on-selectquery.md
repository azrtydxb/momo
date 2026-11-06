# momo-06 Task 13: Preload on SelectQuery

Status: done 2026-11-06
Created: 2026-11-04
Epic: momo-06-relations
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-06-relations Task 13: Preload on SelectQuery (`.procoder/plans/momo-06-relations.md:2818`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-81. Files: `preload.go`, `select.go`, `preload_test.go`. Contributes to S-81; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestPreloadOnQuery` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- `TestPreloadOnQuery` (preload_test.go) written first; red = compile failure (`base.Preload undefined`); green with `go test -tags integration -race -run 'Preload|Load' .` against live PostgreSQL (3 statements: parent + 2 relations; base query unchanged).
- Mutations caught: unclipped append (TestPreloadDerivedQueriesAreIndependent), One skipping preloads (TestPreloadErrorPropagates), Each guard removed (TestPreloadStreamingUnsupported).
- Close-out, `TestPreloadOnQuery` (preload_test.go:16): the base query is not mutated (1 statement, no Author); the derived `Preload(Author, Comments)` query fills both with 3 statements total. Implementation read in preload.go (`Preload` clips the slice, `runPreloads` uses `LoadAll`, `errStreamPreload` guards Each/Iter). The integration run could not be repeated at close (local PostgreSQL resets connections); the run recorded here stands and CI re-runs it. Shipped in commit c744711 (#5).
- `make fmt-check vet staticcheck gosec tidy-check` clean; `procoder check` 0 blocking; commit `feat(relations): Preload on SelectQuery`.
