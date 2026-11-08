# momo-09 Task 9: momotest fixtures and ephemeral SQLite

Status: done 2026-11-08
Created: 2026-11-04
Epic: momo-09-observability-testing-sqlite
Sprint: -

## Description

Implements plan momo-09-observability-testing Task 9: momotest fixtures and ephemeral SQLite (`.procoder/plans/momo-09-observability-testing.md:1520`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-34. Files: `momotest/fixtures.go`, `momotest/sqlite.go`, `momotest/testdata/fixtures.yaml`, `momotest/fixtures_test.go`, `go.mod`. Contributes to S-34; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestMomotestFixtures` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestMomotestSQLiteEphemeral` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

Red: ci-agent run 42466619808 (throwaway branch worktree-agent-aadb520be05ad24c8-red, headSha b0bda384f9725c615c70fa3f217eb37de6215549, tests only on top of sprint-5, branch deleted). unit and PostgreSQL integration failed to build with `undefined: parseFixtures` and `undefined: insertStatement` in momotest, plus the missing `sqlitetest` package.

Green: ci-agent run 42470332826 (headSha fac622492dfa05166e9ec2e031f54933fbedd8fb): lint, unit, fuzz-smoke, PostgreSQL integration, MySQL 8.4, MySQL 9.7 and MariaDB integration all success.

Tests: `momotest/fixtures_integration_test.go` (`TestMomotestFixtures` on every dialect of testopen.Each, `TestMomotestFixturesRowError`), `momotest/fixtures_internal_test.go` (parser and statement unit tests, no database), `momotest/sqlitetest/sqlitetest_integration_test.go` (`TestMomotestSQLiteEphemeral`, `TestSQLiteOptionsAndFixtures`), `internal/repocheck/importgraph_test.go` (`TestMomotestAvoidsSQLite`).

Deviations from the plan, all deliberate:

- `momotest.SQLite` is `sqlitetest.New` in the new package `momotest/sqlitetest`. Importing sqlitedriver from momotest would compile the C sources of SQLite and sqlite-vec into every test binary that uses momotest for PostgreSQL (the stub keeps non-cgo builds compiling, but a default cgo build pays). `TestMomotestAvoidsSQLite` keeps momotest free of driver/sqlitedriver, go-sqlite3 and sqlite-vec. `LoadFixtures` stays in momotest (it only needs momo).
- On a build without cgo or on Windows `sqlitetest.New` skips, as testopen.SQLite does.
- `go.mod` and `go.sum` are unchanged: `go.yaml.in/yaml/v3` was already a direct dependency.
- The plan's fixtures test counts `h.After` entries as SQL; `relfixture.Hook.After` records `QueryEvent.Relation`, not SQL. The test uses `testopen.Each`, `relfixture.CreateSchema` and `obstest.Recorder` (SQL text) instead, and a small recording TB for the bad-file case. Test files are named `*_integration_test.go` and `*_internal_test.go` to follow the integration tag convention.
- `LoadFixtures` parsing is split into `parseFixtures` and `insertStatement` so it is unit-tested without a database, and a row with no columns is rejected. Errors name the file, table and row.
- docs/testing.md gains a LoadFixtures and a sqlitetest section (prose only; the docs fragment cap is unchanged).
