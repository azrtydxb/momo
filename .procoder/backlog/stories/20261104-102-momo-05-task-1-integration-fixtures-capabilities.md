# momo-05 Task 1: Integration fixtures, capabilities and the internal/suite package

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-05-writes-queries Task 1: Integration fixtures, capabilities and the internal/suite package (`.procoder/plans/momo-05-writes-queries.md:41`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: supporting work for the plan. Files: `internal/suite/fixture.go`, `internal/suite/caps.go`, `internal/suite/fixture_test.go`.

## Acceptance criteria

- [x] `TestHarnessCreatesDocuments` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- FAIL before: `go test ./internal/suite/` -> `no non-test Go files in .../internal/suite` (build failed).
- PASS after: `make test-integration-postgres` (MOMO_REQUIRE_INTEGRATION=1, -race) -> `ok internal/suite`, all packages ok; `go test -race -count=1 ./...` ok; go build/vet and `make staticcheck` clean.
- Mutation: `Locking: d.Returning` in CapabilitiesOf made TestCapabilitiesOfRows fail for sqlite and mysql; reverted.
- SQLite and MySQL rows are pinned by TestCapabilitiesOfRows from the dialect description (no drivers yet); Each runs PostgreSQL only until their openers register.
- Gate: `procoder check` 0 blocking findings. Commit 6ee52d8.

