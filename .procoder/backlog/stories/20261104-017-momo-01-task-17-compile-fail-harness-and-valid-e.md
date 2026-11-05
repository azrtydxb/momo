# momo-01 Task 17: Compile-fail harness and valid-example build

Status: done 2026-11-04
Created: 2026-11-04
Epic: momo-01-foundation
Sprint: 001-foundation-of-the-query-core-dialects-schema-base-types

## Description

Implements plan momo-01-foundation Task 17: Compile-fail harness and valid-example build (`.procoder/plans/momo-01-foundation.md:1456`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-4. Files: `testdata/compilefail/wrongtype/main.go`, `testdata/compilefail/wrongtype_any/main.go`, `testdata/valid/basic/main.go`, `compilefail_test.go`. Contributes to S-4; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestCompileFail` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestValidExamplesCompile` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- TestCompileFail and TestValidExamplesCompile red: with testdata/ absent → `--- FAIL: TestCompileFail` (`directory not found` for wrongtype and wrongtype_any) and `--- FAIL: TestValidExamplesCompile` (`stat .../testdata/valid/basic: directory not found`); fixtures restored → both `--- PASS`, `make test-compilefail` prints `ok github.com/azrtydxb/momo`.
- Intended errors: wrongtype fails with `cannot use 42 (untyped int constant) as string value in argument to title.Eq`; wrongtype_any with `cannot use v (variable of interface type any) as string value in argument to title.Eq: need type assertion`; the harness asserts `cannot use`.
- Mutation (spike predates the test): `Eq(v T)` loosened to `Eq(v any)` in expr.go → `TestCompileFail` FAIL (`wrongtype compiled; want a compile error`, `wrongtype_any compiled; want a compile error`); restored with `git checkout expr.go`.
- Gate and commit: procoder check clean (0 unformatted, 0 blocking); `go build ./... && go vet ./... && go test -race ./...` all ok; commit 930c5ef.
