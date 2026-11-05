# momo-01 Task 15: Query immutability under concurrent reuse

Status: done 2026-11-04
Created: 2026-11-04
Epic: momo-01-foundation
Sprint: 001-foundation-of-the-query-core-dialects-schema-base-types

## Description

Implements plan momo-01-foundation Task 15: Query immutability under concurrent reuse (`.procoder/plans/momo-01-foundation.md:1301`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-4. Files: `reuse_test.go`. Contributes to S-4; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestQueryReuseNoLeak` exists as written in the plan, fails under a deliberate mutation of the Where append (`slices.Clip` removed; the spike's code predates the test), and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- `go test -race -run TestQueryReuseNoLeak -count=1 .` prints `ok`; with `slices.Clip` removed from the Where append in select.go the same run reports `WARNING: DATA RACE` (restored with git checkout). The test was strengthened to use a base with spare capacity (three Where calls).
- `go build ./... && go vet ./... && go test -race ./...` pass; `procoder check` gate reports 0 blocking; commit 4b47fa3.
