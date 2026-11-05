# momo-01 Task 18: Test database URL helpers and PostgreSQL containers

Status: done 2026-11-04
Created: 2026-11-04
Epic: momo-01-foundation
Sprint: 001-foundation-of-the-query-core-dialects-schema-base-types

## Description

Implements plan momo-01-foundation Task 18: Test database URL helpers and PostgreSQL containers (`.procoder/plans/momo-01-foundation.md:1562`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-20, S-47. Files: `internal/testdb/testdb.go`, `internal/testdb/isolation_integration_test.go`, `internal/testdb/compose.yaml`, `go.mod`. Contributes to S-20, S-47; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestResolve` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestEnvVarNames` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestSQLitePathIsInTempDir` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestRandomSuffixIsUniqueAndSafe` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestPostgresURLIsolated` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- `TestResolve`, `TestEnvVarNames`, `TestSQLitePathIsInTempDir`, `TestRandomSuffixIsUniqueAndSafe`: `go test ./internal/testdb/ -count=1` before implementing: `internal/testdb/testdb_test.go:15:15: undefined: resolve` (also envPostgres, errSkip); after: `ok  github.com/azrtydxb/momo/internal/testdb`.
- `TestPostgresURLIsolated`: before: `no required module provides package github.com/jackc/pgx/v5` (setup failed; pgx not yet added); after `make db-up` and `MOMO_REQUIRE_INTEGRATION=1 MOMO_TEST_POSTGRES_URL=postgres://postgres:momo@127.0.0.1:5432/postgres?sslmode=disable MOMO_TEST_POSTGRES_APP_URL=postgres://momo_app:momo_app@127.0.0.1:5432/postgres?sslmode=disable go test -tags integration -race ./internal/testdb/ -run TestPostgresURLIsolated -count=1 -v`: `--- PASS: TestPostgresURLIsolated` with all three subtests, `ok`.
- Containers: `docker compose config -q` exit 0; `psql select 1` via postgres17 and via PgBouncer (127.0.0.1:6432) print 1. PgBouncer failed with `AUTH_TYPE: trust` (`server login failed: wrong password type`); the working value `scram-sha-256` is committed (the plan allows changing only that line). `make db-down` run afterwards.
- Gate: `go build ./... && go vet ./... && go test -race ./... -count=1` all `ok`; `launcher.sh check`: `procoder gate: 0 clean, 0 unformatted, 0 unchecked, 3 out of scope, 44 hygiene finding(s) (0 blocking)`; commit e98e68c made. golang.org/x/text raised to v0.43.0 to clear a reported vulnerability in the indirect pgx dependency.
