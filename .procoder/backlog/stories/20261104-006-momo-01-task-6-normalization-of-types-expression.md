# momo-01 Task 6: Normalization of types, expressions and schema equality

Status: done 2026-11-04
Created: 2026-11-04
Epic: momo-01-foundation
Sprint: 001-foundation-of-the-query-core-dialects-schema-base-types

## Description

Implements plan momo-01-foundation Task 6: Normalization of types, expressions and schema equality (`.procoder/plans/momo-01-foundation.md:565`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-1. Files: `schema/normalize.go`, `schema/normalize_test.go`. Contributes to S-1; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestNormalizeType` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestNormalizeExpr` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestEqualIgnoresOrderAndConcurrently` exists as written in the plan, failed before the implementation, and passes now
- [x] `FuzzNormalizeType` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- TestNormalizeType: before implementation `go test ./schema/ -run 'Normalize|Equal' -count=1` printed `undefined: NormalizeType`; after, `go test ./schema/ -run 'Normalize|Equal' -count=1 -v` printed `--- PASS: TestNormalizeType`
- TestNormalizeExpr: same failing run printed `undefined: NormalizeExpr`; after, `--- PASS: TestNormalizeExpr`
- TestEqualIgnoresOrderAndConcurrently: same failing run printed `undefined: Equal`; after, `--- PASS: TestEqualIgnoresOrderAndConcurrently`
- FuzzNormalizeType: failing run above (`undefined: NormalizeType`); `go test -run=^$ -fuzz=FuzzNormalizeType -fuzztime=20s ./schema` printed `PASS` / `ok github.com/azrtydxb/momo/schema 21.463s`
- Gate and commit: `launcher.sh check` printed `procoder gate: 2 clean, 0 unformatted, 0 unchecked, 0 out of scope, 18 hygiene finding(s) (0 blocking)`; `go build ./... && go vet ./... && go test -race ./...` all ok; commit c0db748
