# momo-01 Task 14: Keyset pagination with deterministic tie-breaking

Status: done 2026-11-04
Created: 2026-11-04
Epic: momo-01-foundation
Sprint: 001-foundation-of-the-query-core-dialects-schema-base-types

## Description

Implements plan momo-01-foundation Task 14: Keyset pagination with deterministic tie-breaking (`.procoder/plans/momo-01-foundation.md:1237`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-4. Files: `select.go`, `keyset_test.go`. Contributes to S-4; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestKeysetPagination` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- TestKeysetPagination: before the implementation `go test . -run '^TestKeysetPagination$' -count=1` printed a compile error that SelectQuery[unitDocument] has no field or method After and `FAIL github.com/azrtydxb/momo [build failed]`; after implementing After the same command printed `ok  github.com/azrtydxb/momo`. A temporary check (not committed) showed After on a nullable key returns ErrUnsupported.
- Gate and commit: `go build ./... && go vet ./... && go test -race ./...` all ok; commit 0421baf `feat(momo): keyset pagination with deterministic tie-breaking`; `launcher.sh check` before the commit printed `3 clean, 0 unformatted, 0 unchecked, 0 out of scope` (0 blocking).
