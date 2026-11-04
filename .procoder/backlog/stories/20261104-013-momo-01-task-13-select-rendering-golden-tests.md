# momo-01 Task 13: SELECT rendering golden tests

Status: done 2026-11-04
Created: 2026-11-04
Epic: momo-01-foundation
Sprint: 001-foundation-of-the-query-core-dialects-schema-base-types

## Description

Implements plan momo-01-foundation Task 13: SELECT rendering golden tests (`.procoder/plans/momo-01-foundation.md:1158`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-4. Files: `select_golden_test.go`. Contributes to S-4; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestSelectGolden` exists as written in the plan, failed before the implementation (INNER JOIN and explicit ASC cases), and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

`go test . -run '^TestSelectGolden$'` failed first: the self-join rendered `JOIN` (plan: `INNER JOIN`) and ORDER BY omitted `ASC`. After the fix in select.go and expr.go it passes. `go build ./... && go vet ./... && go test -race ./...` pass, `procoder check` is clean, commit 4067965. The ForUpdate and NULLS gating assertions passed immediately (spike code) and fail under deliberate mutation of the Locking and NullsOrdering guards.


