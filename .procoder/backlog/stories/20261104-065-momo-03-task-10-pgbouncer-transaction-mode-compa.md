# momo-03 Task 10: PgBouncer transaction-mode compatibility with QueryExecModeExec

Status: open
Created: 2026-11-04
Epic: momo-03-drivers-postgresql
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-03-drivers Task 10: PgBouncer transaction-mode compatibility with QueryExecModeExec (`.procoder/plans/momo-03-drivers.md:1158`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-30. Files: `driver/pgxdriver/pgbouncer_test.go`, `internal/testdb/compose.yaml`, `internal/testdb/pgbouncer/pgbouncer.ini`, `internal/testdb/pgbouncer/userlist.txt`.

## Acceptance criteria

- [x] `TestPgBouncerTransactionMode` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-30] `TestPgBouncerTransactionMode` runs the query suite through a PgBouncer container in transaction mode with the simple-protocol option; fails if any statement relies on a named prepared statement.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Red first: the first run of `pgbouncer_test.go` failed (relation does not exist through the pooler because search_path is not applied to every server connection, so the workload tables are now schema-qualified), and `TestPgBouncerRejectsNamedStatements` failed with `QueryExecModeCacheStatement through PgBouncer transaction mode succeeded`. That exposed that the running PgBouncer has `max_prepared_statements = 200` and no `track_extra_parameters`, so the plan's mutation check cannot fail through it. That test was replaced by a wire-level detector.
- Green: `go test -race -count=3 ./driver/pgxdriver -run PgBouncer -v` passed three times: `TestPgBouncerTransactionMode` (8 workers x 25 iterations of transaction, read and batch), `TestPgBouncerNoNamedPreparedStatements`, `TestPgBouncerWireCounterDetectsNamedStatements`, `TestPgBouncerBatchUsesUnnamedStatements`, `TestPgBouncerTransactionScopedState` (`set_config(...,true)` and `pg_advisory_xact_lock` vanish at commit on whichever backend serves next), `TestPgBouncerPinnedTransaction` (advisory lock inside a transaction on a pinned connection).
- Mutation check: switching `TestPgBouncerNoNamedPreparedStatements` to `QueryExecModeCacheStatement` fails with `864 named prepared-statement messages on the wire`; restored afterwards.
- S-30 is only partly met: named statements are detected by parsing the pgx frontend byte stream (Parse, Bind, Describe and Close with a non-empty statement name), which holds whatever the pooler is configured to tolerate. The "runs the query suite" part needs `suite.RunAll` (momo-05 Task 35), which does not exist yet, so that criterion stays unticked.
- Compose: `internal/testdb/pgbouncer/pgbouncer.ini` (`max_prepared_statements = 0`, `track_extra_parameters = search_path`, plain auth) and `userlist.txt` are added and mounted by the `pgbouncer` service in `internal/testdb/compose.yaml`. This was not exercised, because no container may be started or stopped here: the running container still uses the old env-driven config. Recreate it with `docker compose -f internal/testdb/compose.yaml up -d --force-recreate pgbouncer` to apply.
- `procoder check`: 0 blocking findings (info only: pre-existing CI hints, doc-mention notices and a gocyclo note on the test-only wire parser).
- PgBouncer config: mounted files fail on this host (Docker Desktop cannot share /Volumes/DATA) and in CI (services start before checkout), so compose and CI configure the image through its environment instead, including MAX_PREPARED_STATEMENTS=0; the plan records this. The local container was recreated with it (decision in .procoder/ask/decisions.md) and `go test -tags integration -race -count=1 ./driver/... .` passes through it.
