# momo-06 Task 9: Many-to-many through a join table

Status: open
Created: 2026-11-04
Epic: momo-06-relations
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-06-relations Task 9: Many-to-many through a join table (`.procoder/plans/momo-06-relations.md:2412`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-6, S-82. Files: `load.go`, `load_test.go`. Contributes to S-82; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestEagerLoadManyToMany` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-6] `TestEagerLoad*` (one per relation kind incl. composite keys): a counting hook asserts the exact query count, chunking over the bind limit, parent pagination and order preserved without duplicates, NULL-FK parents left empty; fails if loading issues per-parent queries or joins that duplicate parents.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Red first: TestEagerLoadManyToMany failed with `tags by doc: map[1:[go] 2:[db]]` (plain has-many treatment) before the join branch; chunk test failed with `key 0 has 2 values for 1 columns`.
- Green: `go test -tags integration -race -count=1 -run 'ManyToMany|Load|Eager' . ./internal/...` ok against Postgres.
- Tests: TestEagerLoadManyToMany, ...Order, ...SharedCopiesStaleAndNull, TestLoadManyToManyChunksOnJoinKeyWidth, TestLoadManyToManyNoRoomForKey, TestLoadManyToManySoftDeleteAndDuplicateJoinRows. One statement per load (hook count), chunking by join key width, NULL/no-key parents skipped, stale cleared, shared children copied.
- Mutations killed: no ORDER BY, group by child key instead of holders, no join-row dedupe, budget by ChildCols, no soft-delete scope, no NULL-key skip, truncated tuple columns.

- Review fixes (2026-11-06): join-row dedupe now keys on (holder parent key, child primary key) whenever the child has a PK (TestLoadManyToManyDuplicateJoinRowsOnNonPKTarget; restoring the ChildCols==ChildPK condition fails it); TestLoadManyToManyCompositeJoinKeyChunking uses WithMaxParams and asserts per-statement args [4 4 2] (budget by len(ChildCols) fails it).
