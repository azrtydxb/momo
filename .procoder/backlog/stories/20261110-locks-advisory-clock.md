# Locks, advisory keys and clock time (issues #65, #67, #68)

Status: done
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

- [x] `ForNoKeyUpdate()` and `ForKeyShare()` with `NoWait`/`SkipLocked`/`LockOf` render on PostgreSQL; MySQL and SQLite refuse with `ErrUnsupported` (unit tests; PostgreSQL integration test)
- [x] the advisory string helpers take an option for `hashtextextended(key, seed)`; `advisory.Lock/TryLock/Unlock` take session locks on a pinned connection, refuse on a pooled `*DB` (tests)
- [x] `timeop.ClockTimestamp()` and `timeop.StatementTimestamp()` render and run on PostgreSQL; other dialects map or refuse (tests)
- [x] integration test under pgxdriver's default `QueryExecModeExec`: `[]types.UUID` and `[]uuid.UUID` bound to `= ANY($1::uuid[])`, and `[]byte` bound to a jsonb column, round-trip correctly; docs updated

## Evidence

CI: ci-agent run 42776271632, headSha c1eb40efc43d3cd734287445bdbfb1917d456656, all 9 jobs green (unit, lint, fuzz-smoke, bench-module, apidiff-docs, integration postgres, mysql84, mysql97, mariadb). The final head moved on to move the UUID-slice call (see the last criterion); its run id is in the sprint report.

- Criterion 1 (#65): `SelectQuery.ForNoKeyUpdate` and `ForKeyShare` in select.go; rendering and the ErrUnsupported refusal on MySQL and SQLite in select_render.go (`writeLock`). Unit tests `TestNoKeyUpdateAndKeyShareGolden`, `TestNoKeyUpdateAndKeyShareUnsupported`, `TestNoKeyUpdateValidation` (select_lockmodes_unit_test.go); PostgreSQL integration test `TestNoKeyUpdateKeyShareConflicts` (select_lockmodes_integration_test.go) runs the row-lock conflict matrix with NOWAIT.
- Criterion 2 (#67): `advisory.Hash64(seed)` option on `XactLockString`, `TryXactLockString` and the new `LockString`, `TryLockString`, `UnlockString`; `advisory.Lock`, `TryLock`, `Unlock` take a `*momo.DB` from `OnConn` and refuse a pooled or nil one with ErrValidation (new `DB.IsPinned`). Unit tests in advisory/advisory_session_test.go; integration tests `TestHash64MatchesHashTextExtended` and `TestSessionLockOnPinnedConnections` (advisory/advisory_session_integration_test.go).
- Criterion 3 (#68): `timeop.ClockTimestamp` (clock_timestamp(); SYSDATE(6) on MySQL; strftime on SQLite) and `timeop.StatementTimestamp` (statement_timestamp(); MySQL ErrUnsupported). Goldens in timeop/timeop_clock_test.go; `TestClockTimestampAdvancesInTransaction` runs on PostgreSQL.
- Criterion 4: driver/pgxdriver/binding_integration_test.go under the default exec mode. `[]types.UUID` and a `[16]byte` type shaped like google/uuid (the module has google/uuid only as an indirect dependency, and making it direct needs a depNotes entry and a docs row, so the test uses a local mirror type) bind to `= ANY(?::uuid[])` and to `= ANY(?)`, with empty and nil slices. This needed a fix: pgx cannot encode these slices without parameter types, so `convertUUIDSliceArgs` (driver/pgxdriver/uuid_args.go) sends them as an array literal. The #57 agent has the same conversion for `[]types.UUID` in vector_codec.go; the two overlap and the integrator should keep one (this one also covers google/uuid slices). Finding for the Orbit team: a bare `[]byte` bound to a jsonb column is rejected (bytea text, SQLSTATE 22P02); `json.RawMessage` and `string` work, and `TestJSONBBinding` pins both. Docs updated: docs/ai-guide.md, docs/capabilities.md, docs/mysql.md; docs/reference.md regenerated from the CI artifact.
