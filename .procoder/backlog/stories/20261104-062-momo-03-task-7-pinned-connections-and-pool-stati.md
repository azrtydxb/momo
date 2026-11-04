# momo-03 Task 7: Pinned connections and pool statistics on PostgreSQL

Status: open
Created: 2026-11-04
Epic: momo-03-drivers-postgresql
Sprint: -

## Description

Implements plan momo-03-drivers Task 7: Pinned connections and pool statistics on PostgreSQL (`.procoder/plans/momo-03-drivers.md:666`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-90. Files: `driver/pgxdriver/pin.go`, `driver/pgxdriver/pin_test.go`, `onconn.go`, `driver/pgxdriver/onconn_test.go`, `db.go`.

## Acceptance criteria

- [ ] `TestPinnedConn` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestOnConnSessionAndHooks` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-90] `TestPinnedConn` asserts session state set on a pinned connection is visible to later statements on it and the pool stats reflect acquisition; fails if statements hop connections.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

