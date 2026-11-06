# migrate: intermittent integration failure in ./migrate

Status: open
Created: 2026-11-05

## Description

On 2026-11-05, right after merging story 087, one run of
`go test -tags integration -race -count=1 ./migrate/...` failed after 43s
(a clean run takes about 7s); the output was not captured. Thirteen later runs,
including one with -count=8, passed. During the story 087 review,
`TestApplyCancelledStillUnlocks` also failed once and the reviewer attributed it
to contention. The long duration suggests a test waiting on an advisory lock or
a lock timeout. Done means the failing test is identified and the cause is fixed,
not retried away.

## Acceptance criteria

- [x] The failing test is identified from a captured run (loop the package with -count and -v into a file until it reproduces)
- [x] The root cause is named in this task (for example a lock left held by a cancelled test, or a shared-name collision between tests)
- [ ] The fix makes `go test -tags integration -race -count=20 ./migrate/` pass with no failures

## Evidence

Captured runs (loop of `go test -tags integration -race -count=1 -v ./migrate/...`
with a pg_stat_activity / pg_locks poller running beside it):

- Run 5 of 10 failed in ./migrate: `TestApplyCancelledStillUnlocks` took 35.42s,
  with "a cancelled apply must fail" and "testopen: pool.Close blocked for 5s: 1
  connection(s) still acquired (leaked)". The 30s `pg_sleep` ran to completion,
  so the cancel never fired.
- Run 16 of 25 failed in ./migrate: `TestCancelledContextStillRecordsFailure/non-transactional`
  with `relation "momo_migrations" does not exist`: a fixed 500ms cancel landed
  before the history table was created.
- Wait chain reproduced by hand (a transaction that has written to the test
  schema and then sleeps, a `DROP SCHEMA ... CASCADE` behind it): the DROP shows
  `wait_event=relation`, `pg_blocking_pids = {sleeper}`, and stays blocked until
  the sleeper commits. The uncancelled 30s sleep therefore keeps the test schema
  locked, which is how the 30s `drop schema ...: context deadline exceeded`
  appears in the cleanup (opTimeout is 30s).
- Cross-package: `CREATE INDEX CONCURRENTLY` in ./migrate/postgres sits in
  `wait_event=virtualxid` behind every older transaction in the database (it
  has no schema scope), so a leaked long transaction from ./migrate stalls it
  and, with the engine's 5s lock_timeout, fails it. Run 8 of the next loop
  showed this: `TestPostgresObjects` failed "canceling statement due to lock
  timeout" with the CIC blocked on pid of an `INSERT INTO docs (id, emb)`
  transaction from another agent's root-package integration run (not from the
  two packages under test).

Root cause: test bug in `TestApplyCancelledStillUnlocks`. Its watcher goroutine
called `pinned(t, db)` on every 20ms poll. Each call pinned a new pooled
connection that is only released at test end, so a slow start (race detector,
two packages running in parallel) exhausted the pool: the watcher then blocked
in Pin forever, `cancel()` never ran, and Apply ran its 30s `pg_sleep` to the
end inside a transaction that held locks on the test schema. That blocked the
schema DROP (and any CIC in the other package), and left a pinned connection
leaked at pool close. `TestCancelledContextStillRecordsFailure` had a related
timing bug (cancel after a fixed 500ms instead of when the statement runs).

Product code is not at fault: the advisory lock id is
fnv64a("momo:<history schema>.momo_migrations"), so it is scoped per schema and
two apps in one database do not block each other; Lock/unlock/reset/discard
release correctly (TestApplyCancelledStillUnlocks asserts it).

Fix: new `cancelWhenRunning` helper (migrate/helpers_test.go) that pins one
connection once, cancels when a statement carrying a per-test marker comment is
active on the server, and stops its watcher before the connection is released;
both tests use it.

Not fixed (inherent): a CREATE INDEX CONCURRENTLY waits for every other
transaction in the same database, so a long transaction from an unrelated
process sharing the database can still make a CIC test hit lock_timeout. Only a
dedicated database per test process would remove that.

Proof so far: 12 consecutive `-race -count=1 ./migrate/...` runs clean after
the fix; `-count=5 ./migrate/postgres` clean.

Still open: two `-count=20 ./migrate/` runs failed (one hit the 10m default
timeout, one finished in 839s with `-timeout 40m`). Both began with a
database-wide stall, not with the cancel tests: `DROP SCHEMA` and `CREATE
SCHEMA` timing out at 30s in unrelated tests (for example
`TestAutoMigrateSkipsIndexOnSkippedTypeChange`), after which the cancel tests
failed as a consequence. The poller caught the chain during one stall: a
`CREATE TABLE IF NOT EXISTS momo_migrations` commit sat in `IO/WalSync` for
more than 25s holding the migration advisory lock and schema locks, another
session's `INSERT INTO docs (id, emb)` (an HNSW insert from another agent's
run) sat in `LWLock/WALWrite`, autovacuum waited on `WALWrite`, and
`DROP SCHEMA` waited on the CREATE TABLE transaction (`pg_blocking_pids`).
That is WAL fsync saturation on the shared Docker Postgres under load (host
load average about 10), which no test or product change here can remove. The
third criterion stays unticked until `-count=20` passes on a quiet database.
