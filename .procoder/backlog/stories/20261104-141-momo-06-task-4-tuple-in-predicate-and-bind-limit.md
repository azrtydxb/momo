# momo-06 Task 4: Tuple IN predicate and bind-limit budget

Status: open
Created: 2026-11-04
Epic: momo-06-relations
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-06-relations Task 4: Tuple IN predicate and bind-limit budget (`.procoder/plans/momo-06-relations.md:1376`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-6, S-77. Files: `in_tuple.go`, `in_tuple_internal_test.go`. Contributes to S-77; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestTupleInRendering` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-6, part] this story's tests pass for the part of S-6 this task delivers; the full criterion is verified in story 146.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Red first: before in_tuple.go existed, `go test . -run '^TestTupleInRendering$'` failed with `undefined: inTuple`; with the implementation it prints `ok`.
- S-6 part (empty IN renders `1 = 0`, bind-limit budget): `TestTupleInRendering`, `TestTupleInMySQLAndEdges` (unit goldens for PostgreSQL, MySQL, SQLite) and `TestTupleInPostgres` (integration, live PostgreSQL, including a full-budget 65535-key single-column statement) pass under `go test -tags integration -race -count=1 . ./internal/...`.
- Mutation: changing the empty rendering to `1 = 1`, the budget by `+ 1`, and the row-value dialect switch made both unit tests fail; restored and green.
- `go vet ./...`, `go vet -tags integration ./...`, `gofmt -l .` and `go test ./...` are clean.
- Finding: PostgreSQL rejects a composite row-value IN list of 32767 tuples (stack depth limit exceeded, SQLSTATE 54001) although it is within the bind limit, so composite chunking in later tasks should cap tuples per statement below the raw bind budget.
