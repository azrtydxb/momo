# momo-05 Task 16: Explicit write states: omitted, zero, NULL and DEFAULT (S-5)

Status: open
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-05-writes-queries Task 16: Explicit write states: omitted, zero, NULL and DEFAULT (S-5) (`.procoder/plans/momo-05-writes-queries.md:1223`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-5. Files: `write_states_test.go`, `internal/suite/writes.go`, `insert.go`, `update.go`. Contributes to S-5; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestWriteStates` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestWriteStatesInsert` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestWriteStatesMultiRow` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Red first: with the suite bodies and wrappers written before the implementation, `go vet .` failed with `ValuesRows undefined` (type InsertQuery has no such method) at `internal/suite/writes.go:508`. Once it compiled, the unit goldens failed on SQLite (the `DefaultInValues` flag was wrongly true) and on the missing DEFAULT VALUES and Omit-with-Values rules.
- Green: `go test -tags integration -race -count=1 . ./internal/...` against the running PostgreSQL passes. `-v -run TestWriteStates` shows the postgres subtests of TestWriteStates, TestWriteStatesInsert and TestWriteStatesMultiRow passing. `go test ./...`, `go vet ./...`, `go vet -tags integration ./...` and `gofmt -l .` are clean.
- Unit goldens for PostgreSQL, SQLite and MySQL are in `write_states_unit_test.go`: single-row and multi-row DEFAULT, per-row DEFAULT on PostgreSQL, ErrUnsupported on SQLite for mixed rows, DEFAULT VALUES, Omit with Values, the ValuesRows bind-parameter limit, and one clock read per statement.
- Deviation from the plan text: the plan runs the bodies on `Document`, which does not map the `status` column, so Values and Update refuse it. The bodies use a new fixture model `suite.StatusDocument` over the same table instead. The SQLite dialect flag `DefaultInValues` was true and is now false, as the plan's rules require.

