# momo-06 Task 5: Eager loading core for belongs-to and has-one, relation hook events

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-06-relations
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-06-relations Task 5: Eager loading core for belongs-to and has-one, relation hook events (`.procoder/plans/momo-06-relations.md:1538`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-6, S-18. Files: `loader.go`, `load.go`, `relhook.go`, `db.go`, `load_test.go`. Contributes to S-18; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestEagerLoadBelongsTo` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestEagerLoadHasOne` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestRelationHookEvents` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-6, part] this story's tests pass for the part of S-6 this task delivers; the full criterion is verified in story 146.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Red first: before the implementation, `go test . -run 'TestEagerLoad|TestRelationHook'` failed to build with `undefined: momo.Load` and `h.Events undefined`. Now `TestEagerLoadBelongsTo`, `TestEagerLoadHasOne` and `TestRelationHookEvents` pass on PostgreSQL under `go test -tags integration -race -count=1 . ./internal/...`.
- S-6 part: unit tests in `load_internal_test.go` cover dedupe, NULL keys never queried, first-seen order, stable association for duplicate parents, stale-association clearing, the has-one over-match error, and bind-limit chunking with order preserved and scope args reserved. `TestEagerLoadLargeCompositeKeySet` (integration) loads 20000 composite keys: with the composite cap disabled it failed with SQLSTATE 54001; with the cap from story 141 (2000 tuples per statement) it issues 10 statements and passes.
- Mutation: always appending keys (no dedupe), widening each chunk by one, and prepending instead of appending children each made a unit test fail; restored and green.
- `RelationEvent`, `RelationHook` and event recording in `relfixture.Hook` were added (`relhook.go`, `db.go`, `internal/relfixture/schema.go`).
- `go vet ./...`, `go vet -tags integration ./...`, `gofmt -l .` and `go test ./...` are clean.
- Review fixes: the scope-arg reservation is now pinned by `TestLoadReservesUserScopeArgs` (LoadWhere with 20 values, MaxParams 40, 30 parents: 8 statements of 4 keys, none over the limit); changing `reserved := 16 + scopeArgs` to `16` makes it fail. A scope that leaves no room for even one key under MaxParams now returns an `ErrValidation` (Op "load") before any statement runs.
- Soft delete: Load excludes soft-deleted children by default, like Select (the spec states no relation-specific rule, so Select's behavior is the reference). `LoadWithDeleted` and `LoadOnlyDeleted` mirror `WithDeleted` and `OnlyDeleted`. Covered by a unit golden and an integration test (`TestLoadSoftDeletedChildren`, PostgreSQL) with a soft-deleted child.
- Also covered: int32 parent key against int64 child FK (making `keyString` type-sensitive fails it), context cancellation between chunks (ctx error, event with Err, associations cleared), every chunk on the passed tx, event contents on error, scan and iteration errors classified with Op "load". `LoadPerParentLimit` now returns `ErrUnsupported` until the top-N task (plan task 12) implements it. Docs updated for up-front clearing, per-parent shallow copies and when relation events fire.
