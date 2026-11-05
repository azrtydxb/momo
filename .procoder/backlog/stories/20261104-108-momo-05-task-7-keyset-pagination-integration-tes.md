# momo-05 Task 7: Keyset pagination integration tests (consumes momo-01 After)

Status: open
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-05-writes-queries Task 7: Keyset pagination integration tests (consumes momo-01 After) (`.procoder/plans/momo-05-writes-queries.md:532`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: supporting work for the plan. Files: `internal/suite/keyset.go`, `integration_keyset_test.go`.

## Acceptance criteria

- [x] `TestKeysetPaginationMixedDirections` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestKeysetRejectsNullableKey` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Red: `go vet -tags integration .` with only the wrapper test file failed with `undefined: suite.KeysetPaginationMixedDirections`. Green: after adding `internal/suite/keyset.go`, `go test -tags integration -race -count=1 -run Keyset .` passes on PostgreSQL for both tests (ten rows, pages of three, no repeats, at least four pages; nullable key gives ErrUnsupported). `After` in `select.go` needed no fix.
- Extra unit golden `TestKeysetMixedDirectionsGoldenSQLite` covers DESC then ASC then pk, bound args and the soft-delete filter. MySQL has no dialect yet, so no MySQL golden.
- Gate: go vet ./..., go vet -tags integration ., gofmt -l, go test ./... and the full integration run are clean.
