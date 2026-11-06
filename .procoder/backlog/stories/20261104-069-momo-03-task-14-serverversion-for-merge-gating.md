# momo-03 Task 14: ServerVersion for MERGE gating

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-03-drivers-postgresql
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-03-drivers Task 14: ServerVersion for MERGE gating (`.procoder/plans/momo-03-drivers.md:1617`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: supporting work for the plan. Files: `driver/pgxdriver/version.go`, `driver/pgxdriver/version_test.go`.

## Acceptance criteria

- [x] `TestServerVersion` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Red first: `version_test.go` was written before `version.go`. Run: `go test ./driver/pgxdriver -run TestServerVersion` failed with the panic `interface conversion: *pgxdriver.driver is not momo.ServerVersioner: missing method ServerVersion`.
- Green: after adding `version.go`, `go test -race -count=1 ./driver/pgxdriver -run TestServerVersion -v` passed `TestServerVersion`, `TestServerVersionSQLDB` and `TestServerVersionSQLDBRejectedArgument` against the running pgvector pg17 container (server_version_num is at least 150000).
- `ServerVersion` is implemented on the native pgxpool driver (default exec mode, so PgBouncer-safe) and on the `FromSQLDB` adapter, which returns the rejection error for a nil or non-pgx `*sql.DB`. Both carry compile-time `momo.ServerVersioner` assertions.
- Full run: `go test -tags integration -race -count=1 ./driver/... .` with the integration env passed; `go vet ./...`, `go vet -tags integration ./...` and `gofmt -l .` are clean.
- `procoder check`: 0 blocking findings (info only: pre-existing CI hints, doc-mention notices and a gocyclo note on the test-only wire parser).
- Fresh review: ServerVersion uses current_setting(server_version_num) so it reports the backend version through PgBouncer; signature matches momo.ServerVersioner; no blocking findings.
