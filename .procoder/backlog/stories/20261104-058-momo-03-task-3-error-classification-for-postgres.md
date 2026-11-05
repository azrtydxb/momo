# momo-03 Task 3: Error classification for PostgreSQL (pgx) with value redaction at the source

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-03-drivers-postgresql
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-03-drivers Task 3: Error classification for PostgreSQL (pgx) with value redaction at the source (`.procoder/plans/momo-03-drivers.md:268`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: supporting work for the plan. Files: `driver/pgxdriver/classify.go`, `driver/pgxdriver/classify_test.go`.

## Acceptance criteria

- [x] `TestClassifyUniqueRedactsDetail` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestClassifySerialization` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- FAIL before implementation: `go test ./driver/pgxdriver/ -run TestClassify` -> `undefined: ClassifyError` (build failed).
- PASS after: `go test -race -count=1 ./driver/pgxdriver/` -> ok. Also covered: full SQLSTATE mapping, passthrough, runtime-built secret absent from Error()/%v/%+v/Detail().
- Mutation check: copying pe.Detail into the wrapped message made TestClassifyUniqueRedactsDetail FAIL; reverted, PASS.
- Gate: `go build`, `go vet`, `go test -race -count=1 ./...`, `make tidy-check` pass; procoder check: 0 blocking findings.
- Commit: feat(pgx): classify PostgreSQL errors without leaking detail
