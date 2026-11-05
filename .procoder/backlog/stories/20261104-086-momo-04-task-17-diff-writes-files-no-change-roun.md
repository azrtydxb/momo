# momo-04 Task 17: Diff writes files; no-change round trip (PostgreSQL)

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Note (Task 9 review): use `migrate.OrderErr` (or `OrderWithViews`), not `Order`, and surface the cycle error it returns.

Implements plan momo-04-migrations Task 17: Diff writes files; no-change round trip (PostgreSQL) (`.procoder/plans/momo-04-migrations.md:2036`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-10, S-11, S-27. Files: `migrate/engine.go`, `migrate/diff_engine_test.go`. Contributes to S-10, S-11, S-27; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestMigrationRoundTrip` exists as written in the plan, failed before the implementation, and passes now
- [x] The no-change round trip ignores the `momo_migrations` history table (the engine Inspect used by Diff excludes it)
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

- Red first: before the implementation `go vet ./migrate/` failed with `m.Diff undefined (type *migrate.Engine has no field or method Diff)`.
- `TestMigrationRoundTrip` (Diff, Apply, Inspect, `schema.Equal`, second Diff returns nil) and `TestNoChangeWritesNothing` pass against Postgres: `go test -tags integration -race -count=1 ./migrate/...` gives ok for migrate and migrate/postgres.
- `TestInspectIgnoresHistoryTable` applies a migration so `momo_migrations` exists, then asserts `Engine.Inspect` omits it. Mutation check: with the filter disabled, `TestInspectIgnoresHistoryTable` and `TestMigrationRoundTrip` both fail.
- Offline unit tests: `TestDiffFilesAndHeader`, `TestDiffSnapshotParentAndSum`, `TestDiffDestructiveHeader`, `TestDiffNoTransactionHeader`, `TestDiffRejectsBadNameAndBumpsClash`, `TestDiffSurfacesOrderCycle` (view cycle surfaces as an error from OrderWithViews).
- `go vet ./...` clean, `gofmt -l .` empty, `go test ./...` ok.
- Note: `Engine.Diff` itself reads the snapshot, not the live database, so the history table cannot enter it; the exclusion lives in `Engine.Inspect`, which the round trip and later drift and baseline use.
- Deviation: the version comes from `NextVersion` (clock, bumped past the latest existing version) instead of the bare clock, and the test schema declares the `vector` extension because the test database has it installed.
