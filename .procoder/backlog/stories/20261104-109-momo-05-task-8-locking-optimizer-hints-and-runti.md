# momo-05 Task 8: Locking, optimizer hints and runtime table override (S-79)

Status: open
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-05-writes-queries Task 8: Locking, optimizer hints and runtime table override (S-79) (`.procoder/plans/momo-05-writes-queries.md:583`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-79. Files: `integration_locking_test.go`, `select.go`. Contributes to S-79; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestLockingNowaitSkipLocked` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestWithTableNameQuoted` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestHintsByDialect` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

- Red first: with select.go at HEAD (no IndexedBy or WithTableName), `go vet -tags integration .` failed to compile: internal/suite/queries.go: WithTableName undefined, IndexedBy undefined. Unit golden TestLockingHintsTableNameGolden failed to compile the same way before the implementation. The integration tests live as thin wrappers in integration_locking_test.go over suite members LockingNowaitSkipLocked, WithTableNameQuoted and HintsByDialect in internal/suite/queries.go (also run by Queries in run.go).
- Green: `go test -tags integration -race -count=1 . ./internal/...` all ok against the running Postgres (including the two-transaction SKIP LOCKED and NOWAIT check, NOWAIT yields ErrBusy); go vet (both tag sets), gofmt and go test ./... clean.
- Deviation: the plan has no FOR NO KEY UPDATE, KEY SHARE, OF or runtime-settings API in Task 8 (ForUpdate and ForShare already existed; SetLocal exists in search.go), so none were added.

- Fresh review: Nothing found. Plan Task 8 specifies only IndexedBy and WithTableName (locking already existed); dropping the WithTableName alias and swapping NOWAIT/SKIP LOCKED are both killed by tests.
