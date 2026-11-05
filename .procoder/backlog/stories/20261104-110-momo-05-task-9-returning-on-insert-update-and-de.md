# momo-05 Task 9: RETURNING on insert, update and delete

Status: open
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-05-writes-queries Task 9: RETURNING on insert, update and delete (`.procoder/plans/momo-05-writes-queries.md:691`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: supporting work for the plan. Files: `returning_test.go`.

## Acceptance criteria

- [x] `TestReturningRendersAndScans` exists as written in the plan, failed before the implementation, and passes now
- [x] `DeleteQuery` Returning, ReturningAll and Rows call the shared `renderReturning` and `runReturning`, with a test
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- FAIL (before implementation): `go vet .` failed with `momo.UpdateQuery[suite.Document] has no field or method Returning` (returning_test.go).
- PASS: `MOMO_REQUIRE_INTEGRATION=1 ... go test -tags integration -race -count=1 .` gave `ok`; `TestReturningRendersAndScans` (returning_integration_test.go, as written in the plan) passes on postgres, as do `TestReturningOnUpdateScansInColumnOrder`, `TestReturningRendersPerDialect`, `TestReturningUnsupportedOnMySQL` and `TestReturningValidation`.
- Mutation check: replacing the dialect gate in `renderReturning` with `false` made `TestReturningUnsupportedOnMySQL` fail (`update err = <nil>, want ErrUnsupported`); restored.
- Insert and Update call the shared `renderReturning`/`runReturning` in returning.go; `ReturningAll()` added to both (plan text corrected: Task 12 did not add it). Rows on a version-checked Model update is refused with ErrValidation (use Exec).
- Delete side (story 118 branch): delete.go calls renderReturning/runReturning; TestDeleteReturningRenders and TestDeleteReturningUnsupportedOnMySQL (unit) and TestDeleteReturningRows (postgres, integration) pass with -race.
- `make staticcheck`, `go vet ./...`, `go test -race -count=1 .` clean.
