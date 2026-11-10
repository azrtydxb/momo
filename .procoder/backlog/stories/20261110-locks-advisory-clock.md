# Locks, advisory keys and clock time (issues #65, #67, #68)

Status: open
Created: 2026-11-10
Epic: momo-05-writes-queries
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

Kuvryn Orbit needs three related things:
- #65: `FOR NO KEY UPDATE` and `FOR KEY SHARE`, at 9 sites that must not block FK inserts.
- #67: 64-bit `hashtextextended(key, seed)` advisory keys and session-level locks. 18 of Orbit's 20 lock sites use them, and its migration runner takes a session lock.
- #68: `clock_timestamp()` and `statement_timestamp()`, used at about 47 lease and claim sites.

The Orbit session also asked for proof that `[]uuid.UUID` binds to `ANY($1::uuid[])`, and `[]byte` to jsonb, under pgxdriver's default exec mode.

## Acceptance criteria

- [ ] `ForNoKeyUpdate()` and `ForKeyShare()` with `NoWait`/`SkipLocked`/`LockOf` render on PostgreSQL; MySQL and SQLite refuse with `ErrUnsupported` (unit tests; PostgreSQL integration test)
- [ ] the advisory string helpers take an option for `hashtextextended(key, seed)`; `advisory.Lock/TryLock/Unlock` take session locks on a pinned connection, refuse on a pooled `*DB` (tests)
- [ ] `timeop.ClockTimestamp()` and `timeop.StatementTimestamp()` render and run on PostgreSQL; other dialects map or refuse (tests)
- [ ] integration test under pgxdriver's default `QueryExecModeExec`: `[]types.UUID` and `[]uuid.UUID` bound to `= ANY($1::uuid[])`, and `[]byte` bound to a jsonb column, round-trip correctly; docs updated
