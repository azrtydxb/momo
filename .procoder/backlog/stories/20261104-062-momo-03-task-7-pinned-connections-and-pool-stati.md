# momo-03 Task 7: Pinned connections and pool statistics on PostgreSQL

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-03-drivers-postgresql
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-03-drivers Task 7: Pinned connections and pool statistics on PostgreSQL (`.procoder/plans/momo-03-drivers.md:666`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-90. Files: `driver/pgxdriver/pin.go`, `driver/pgxdriver/pin_test.go`, `onconn.go`, `driver/pgxdriver/onconn_test.go`, `db.go`.

## Acceptance criteria

- [x] `TestPinnedConn` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestOnConnSessionAndHooks` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-90] `TestPinnedConn` asserts session state set on a pinned connection is visible to later statements on it and the pool stats reflect acquisition; fails if statements hop connections.
- [x] `TestDefaultExecModeNoNamedStatements` (story 059) runs without a skip once `Pin` lands: remove its `TODO(momo-03 Task 7)` `t.Skip` in `driver/pgxdriver/driver_test.go`
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Red: `go test ./driver/pgxdriver/ -run 'Pinned|OnConn'` failed to build (`undefined: errReleased`, `db.OnConn undefined`; no `Pin` on the driver). Green: `make test-integration-postgres` (MOMO_REQUIRE_INTEGRATION=1, race) passes against the Momo compose Postgres; `go build`, `go vet`, `go test -race ./...` and `make staticcheck` pass.
- [S-90] `TestPinnedConn` sets `application_name` on the pinned connection and reads it back, checks `AcquiredConns >= 1` while pinned and 0 after Release. `TestPinnedStatementsShareOneBackend` additionally churns the pool and compares `pg_backend_pid()` across Exec, Query and the pinned transaction.
- Story 059: `TestDefaultExecModeNoNamedStatements` no longer skips (the `TODO(momo-03 Task 7)` `t.Skip` and the `ok` check are removed) and passes, so story 059 closes in a following commit.
- Mutation: removing `p.conn = nil` from `pinned.Release` makes `TestPinnedUseAfterReleaseFails` fail (nil-pointer panic from pgxpool instead of `errReleased`); restored.
- Release: returns the connection exactly once; a second Release returns nil and cannot free a connection the pool has since handed out (`TestPinnedDoubleReleaseReturnsConnectionOnce`); use after Release returns an error, never panics; Release with an open pinned tx is discarded by the pool without leaking.
- Pinned `Begin` reuses the `*pgxTx` wrapper (asserted in `TestPinnedTxReusesTxWrapper`); all errors go through `ClassifyError`; `DB.Begin`/`DB.Tx` on an `OnConn` handle return `ErrUnsupported` (Op "begin").
- Deviations: `internal/testdb/testopen` (story 056) is not on this branch, so `onconn_test.go` builds its `*momo.DB` through a local `openDB` helper (pgxdriver.NewPool + guarded pool close) that testopen replaces; the pool wait test now holds the connection with `Pin` as the plan says; the root `execer` has no replicas field in this tree, so `OnConn` copies the DB and rebinds the executor only.
- `procoder check`: 0 blocking findings.
