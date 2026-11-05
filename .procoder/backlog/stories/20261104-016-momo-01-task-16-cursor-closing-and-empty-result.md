# momo-01 Task 16: Cursor closing and empty-result semantics

Status: done 2026-11-04
Created: 2026-11-04
Epic: momo-01-foundation
Sprint: 001-foundation-of-the-query-core-dialects-schema-base-types

## Description

Implements plan momo-01-foundation Task 16: Cursor closing and empty-result semantics (`.procoder/plans/momo-01-foundation.md:1365`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-4, S-8. Files: `cursor_test.go`.

## Acceptance criteria

- [x] `TestIterClosesCursor` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestEmptyResultNotNotFound` exists as written in the plan, fails under a deliberate mutation of One and All (the spike's code predates the test)
- [x] [S-4, part] `TestIterClosesCursor` passes for the part of S-4 this task delivers; the full criterion is verified in story 067.
- [x] [S-8, part] `TestEmptyResultNotNotFound` passes for the part of S-8 this task delivers; the full criterion is verified in story 200.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- TestIterClosesCursor: before the implementation `go test . -run 'TestIterClosesCursor|TestEmptyResultNotNotFound' -count=1 -v` printed `cancel mid-iteration: err=<nil> closed=2, want context.Canceled and 1` and `--- FAIL: TestIterClosesCursor`; after adding the ctx.Err check before each Next in Each and Iter it printed `--- PASS: TestIterClosesCursor`.
- TestEmptyResultNotNotFound: passed before the implementation. With One returning a nil error on zero rows it printed `One on zero rows = <nil>, want ErrNotFound`; with All returning ErrNotFound on zero rows it printed `All on zero rows = ([], momo: not found (op=select)), want empty and nil error`; both mutations were restored and the test printed `--- PASS`.
- [S-4, part] and [S-8, part]: both tests pass in `go test -race ./...`; the full criteria are verified in stories 067 and 200.
- Gate and commit: `go build ./... && go vet ./... && go test -race ./...` all ok after merging init; commit 86d0733 `test(momo): cursors close on every return path and empty results are not errors`, merge commit ae6e852; `launcher.sh check` printed `0 blocking`.
