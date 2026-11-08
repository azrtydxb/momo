# momo-04 Task 36: SQLite session setup and migration lock

Status: closed
Created: 2026-11-04
Epic: momo-04-migrations-sqlite
Sprint: -

## Description

Implements plan momo-04-migrations Task 36: SQLite session setup and migration lock (`.procoder/plans/momo-04-migrations.md:3933`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-11, S-13, S-57. Files: `migrate/sqlite/lock.go`. Contributes to S-11, S-13, S-57; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestSQLiteLockTable` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestSQLiteForeignKeysOffDuringRebuild` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Tests (migrate/sqlite/lock_test.go, integration): `TestSQLiteLockTable`, `TestSQLiteForeignKeysOffDuringRebuild`, plus `TestSQLiteUnlockOnlyOwnRow`, `TestSQLiteForceUnlockWithoutTable`, `TestSQLiteSetupWithoutForeignKeysOffChangesNothing`; lock and restore through `Engine.Apply` in `TestSQLiteEngineLifecycle` (lock row removed, foreign keys back on) and `TestSQLiteEngineRebuildOrphanFailsAndRestores`.
- Red: ci-agent run 42467369597 (headSha 374d20a44bafd45aaeb802b85aca4de0ad0b87ab) on the throwaway branch worktree-agent-a2047783134c30958-red, which held only the tests on top of sprint-5 (deleted afterwards): unit, lint and every integration job failed to build (`undefined: tokenize`, `undefined: parseTableSQL`, `undefined: enumCheck`, `undefined: msqlite.New`, `migrate.SessionOptions` has no field `ForeignKeysOff`).
- Green: ci-agent run 42470149633 (headSha 014329032c0f3539e0166560196c0ca52d63a513) on worktree-agent-a2047783134c30958: lint, unit (make test, test-compilefail, test-race), fuzz-smoke, PostgreSQL integration and MySQL 8.4, MySQL 9.7 and MariaDB integration all success. The integration jobs run `go test -tags integration -race ./...`, which includes migrate/sqlite against a real SQLite (earlier green run 42468332372, headSha a341598629adf51ba8e20f09601a9e5b2ed28ee6, same code).
- Per-test output: ci-agent run 42470163697 (headSha e98500bce10e87834c069617267450b55309d36f) on the throwaway branch worktree-agent-a2047783134c30958-verbose (deleted afterwards), the same code with the PostgreSQL integration target narrowed to `go test -v -tags integration -race ./migrate/sqlite/`: 51 PASS lines, 0 FAIL, 0 SKIP; every test named below shows PASS.
- Gate: the commit hook ran `procoder check` with 0 blocking findings.

### Deviations from the plan

- `SessionOptions` gained `ForeignKeysOff bool` in migrate/types.go (the plan said Task 3's file).
- `TestSQLiteLockTable` opens its database with `sqlitedriver.Open(..., Config{MaxOpenConns: 2})` instead of `testopen.SQLite`: that pool has one connection, so two pinned sessions would wait for each other forever. The assertions are the plan's.
- `Setup` reads `PRAGMA foreign_keys` back and fails when the value did not change (the pragma is a silent no-op inside a transaction); `reset` is idempotent, ignores cancellation of ctx and discards the connection when the restore fails. The lock and statement timeouts of `SessionOptions` are ignored (busy handling is the driver's busy_timeout).
- `Lock` ignores `key` (a SQLite file has one history), maps `momo.ErrUniqueViolation` (not raw sqlite codes 1555 and 2067) to `ErrLocked`, and `unlock` returns an error when the row was no longer this holder's (removed by `ForceUnlock` or taken over).
- `ForceUnlock` has no CLI flag; `momo migrate` still does not select the SQLite dialect (no story asked for it).
- `migrate.isUndefinedTable` also recognises SQLite's "no such table" message, which has no SQLSTATE, so `Plan`, `Status` and `Apply` treat a missing history table as an empty history; `Engine.readHistoryAt` uses it.
