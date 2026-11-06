# momo-03 Task 6: Transactions with isolation, read-only access and savepoints on PostgreSQL

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-03-drivers-postgresql
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-03-drivers Task 6: Transactions with isolation, read-only access and savepoints on PostgreSQL (`.procoder/plans/momo-03-drivers.md:603`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: supporting work for the plan. Files: `driver/pgxdriver/driver.go`, `driver/pgxdriver/tx_test.go`.

## Acceptance criteria

- [x] `TestTransactionsIsolationReachesServer` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestUnknownIsolationIsUnsupported` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Red: not available in the strict sense. Task 4 on sprint-2 already ships `Begin`, `Tx` and the isolation and read-only mapping, so the two plan tests (`TestTransactionsIsolationReachesServer`, `TestUnknownIsolationIsUnsupported`) passed on first run. Their fault detection is shown by mutation instead. Green: `go test -race` on the Momo compose Postgres.
- Mutation: mapping `sql.LevelRepeatableRead` to `pgx.ReadCommitted` fails `TestTransactionsIsolationReachesServer` (`transaction_isolation = "read committed"`) and the all-levels test; restored.
- Extra tests: every isolation level and access mode reaches the server, a failing nested savepoint scope keeps the outer tx usable and commits the outer work, two-deep savepoints use distinct generated names and discard inner work with the failing middle scope, a hostile savepoint name is quoted (no injection), Savepoint after Commit is ErrTxDone.
- `driver.go` is unchanged: the existing `Begin`/`pgxTx` is extended by tests only; savepoints are issued by the root `Tx.Savepoint` through `Exec` with quoted, generated names (`momo_sp_<depth>`).
- `procoder check`: 0 blocking findings. `make staticcheck` clean.
