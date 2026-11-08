# momo-03 Task 22: Pure-Go SQLite adapter (modernc, no cgo, no vectors)

Status: done 2026-11-08
Created: 2026-11-04
Epic: momo-03-drivers-sqlite
Sprint: -

## Description

Implements plan momo-03-drivers Task 22: Pure-Go SQLite adapter (modernc, no cgo, no vectors) (`.procoder/plans/momo-03-drivers.md:2179`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: supporting work for the plan. Files: `driver/sqlitepure/open.go`, `driver/sqlitepure/open_test.go`, `go.mod`.

## Acceptance criteria

- [x] `TestPureSQLiteCapabilitiesAndErrors` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

Red: red run 42466012379 on 0261a54d9b5091ec6d4053b47d088c198aae0867 (tests only on top of sprint-5, throwaway branch worktree-agent-aa450d3e0da3c4a3b-red, deleted afterwards; all seven jobs failed); unit and integration logs show `undefined: Config`, `undefined: buildDSN`, `undefined: checkVersion` in driver/sqlitepure. Green: ci-agent run 42466819006 on 7d7faa67e3d4b835a4b7800b03ce5d0b86b5186c (lint, fuzz-smoke, unit, integration postgres, mysql84, mysql97, mariadb all success); the integration job log has `ok github.com/azrtydxb/momo/driver/sqlitepure` under -race with the integration tag (TestPureSQLiteCapabilitiesAndErrors and the constraint-kind, busy, transaction, read-only, pinned, Columns, time, redaction and DSN tests). Commits: 0261a54 (tests), 13e6b3e (adapter), 6704608 (go.mod and go.sum, produced by `go get modernc.org/sqlite@v1.60.1` and `go mod tidy` on a throwaway CI branch, tmp-modernc-tidy, run 42466073536, deleted afterwards). The procoder commit gate ran on each commit and was clean.

Deviations from the plan task (adapted to the code as it is on sprint-5):

- Package path `driver/sqlitepure`, as the plan names it. Config is not imported from `driver/sqlitedriver`: that package links mattn/go-sqlite3 and sqlite-vec whenever cgo is on, which would defeat the no-cgo import graph. The type lives in `internal/sqliteconf`; `sqlitedriver.Config` and `sqlitepure.Config` are aliases of it, so they are one type (compile-time check in driver/config_identity_test.go).
- Code sharing: the cgo package's driver.go, pin.go, readonly.go and timescan.go carry `cgo && !windows` build tags and use mattn types, so they cannot be imported; sqlitepure has adapted copies with the same behaviour (read-only transactions through PRAGMA query_only and BEGIN DEFERRED, pinning with Release and Discard, Columns, redacted scan errors, time scanners, the single-connection default). Only Config is shared.
- DSN: `_pragma` busy_timeout, journal_mode(WAL), synchronous, foreign_keys(1) and `_txlock` are set as the plan says. The caller's query may carry only `_txlock`, `mode` and `cache` (allowlist, with the same refusals as the cgo package for mode and cache on plain paths and for a private in-memory database with more than one connection); `_pragma` and every other key are refused so a DSN cannot weaken the safety settings, and the busy timeout comes only from Config. Added `_time_format=sqlite` so modernc writes time.Time in the layout the scanners read; the plan did not mention it.
- Open pings, then checks `sqlite_version() >= 3.35.0` once. There is no per-connection hook and no load_extension probe: modernc.org/sqlite cannot load extensions, and its connection hook is process-global, which this package avoids.
- Classification: as in the plan (BUSY and LOCKED by `Code()&0xff` to ErrBusy; 2067 and 1555 to unique; 787; 275; 1299), plus SQLITE_BUSY_SNAPSHOT (517) to ErrSerialization as in the cgo package, a message-prefix fallback for a bare SQLITE_CONSTRAINT (19), and the " (code)" suffix modernc appends is dropped before Table and Column are parsed.
- Tests: DB tests carry the `integration` tag (repo convention) and use `Config{}` instead of `sqlitedriver.Config{}`; the DSN, parser and version tests are untagged unit tests; three fuzzers were added to FUZZ_TARGETS in the Makefile.
