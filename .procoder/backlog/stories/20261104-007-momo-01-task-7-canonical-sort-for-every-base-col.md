# momo-01 Task 7: Canonical Sort for every base collection

Status: done 2026-11-04
Created: 2026-11-04
Epic: momo-01-foundation
Sprint: 001-foundation-of-the-query-core-dialects-schema-base-types

## Description

Implements plan momo-01-foundation Task 7: Canonical Sort for every base collection (`.procoder/plans/momo-01-foundation.md:671`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-1. Files: `schema/schema.go`, `schema/sort_test.go`.

## Acceptance criteria

- [x] `TestSortCoversAllCollections` exists as written in the plan, fails under a deliberate mutation of `Sort` (the spike's `Sort` predates the test), and passes on the restored code
- [x] `FuzzSchemaSort` exists as written in the plan, fails under a deliberate mutation of `Sort` (the spike's `Sort` predates the test), and passes on the restored code
- [x] [S-1] `TestSchemaJSONRoundTrip` and `FuzzSchemaSort` (schema package): JSON round-trips losslessly and `Sort` makes equivalent schemas byte-identical; fails if a field is dropped from serialization or Sort is order-sensitive.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- TestSortCoversAllCollections: `go test ./schema -run TestSortCoversAllCollections` with the Checks sort deleted, then with the Tables sort deleted, printed `--- FAIL: TestSortCoversAllCollections ... Sort left a collection unsorted` both times; after `git checkout schema/schema.go` `go test ./schema -v` printed `--- PASS: TestSortCoversAllCollections`.
- FuzzSchemaSort: with the Tables sort deleted, `go test -run='^$' -fuzz=FuzzSchemaSort -fuzztime=30s ./schema` printed `--- FAIL: FuzzSchemaSort ... Sort is order-sensitive`; on restored code the same command printed `PASS` (3392557 execs, ok github.com/azrtydxb/momo/schema 30.558s).
- [S-1]: `go test ./schema -run 'TestSchemaJSONRoundTrip|TestSortCoversAllCollections' -v` printed `--- PASS: TestSchemaJSONRoundTrip`; FuzzSchemaSort passed as above and failed under the Tables mutation (order-sensitive Sort detected).
- Gate and commit: `go build ./... && go vet ./... && go test -race ./...` all ok; commit d430e50 `test(schema): cover canonical sort for every base collection`; `launcher.sh check` printed `procoder: no changed files` after the commit (gofmt had been applied to schema/sort_test.go before it).
