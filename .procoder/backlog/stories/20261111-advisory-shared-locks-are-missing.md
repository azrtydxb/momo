# advisory shared locks are missing (pg_advisory_xact_lock_shared and friends)

Status: done 2026-11-11
Created: 2026-11-11
Epic: -
Sprint: 009-fix-every-open-github-issue-86-to-108-the-kuvryn-orbit-port
Type: bug
Severity: s2

## Description

GitHub issue #94. The `advisory` package has exclusive transaction and session
locks only. There is no `pg_advisory_xact_lock_shared`,
`pg_try_advisory_xact_lock_shared`, `pg_advisory_lock_shared`,
`pg_try_advisory_lock_shared` or `pg_advisory_unlock_shared` helper, so
Kuvryn-Owl's reader/writer pair on one key (object writers take the shared lock,
the reconciler takes the exclusive one) cannot move to momo with identical
keys.

Reproduction: search the module for `lock_shared` (no hit), or try to write
`advisory.XactLockShared(ctx, tx, key)` (does not compile).

Observed: no shared helper exists. Expected: `XactLockShared`,
`TryXactLockShared`, `LockShared`, `TryLockShared`, `UnlockShared` and the
`...String` forms, taking the same `Option`s, so `Hash64(52)` reproduces
`hashtextextended($1::text, 52)`.

Root cause: the package was built from the exclusive functions only; the shared
variants were never added.

## Acceptance criteria

- [x] `advisory.XactLockShared`, `TryXactLockShared`, `LockShared`, `TryLockShared`, `UnlockShared` and their `...String` forms exist and render `pg_advisory_xact_lock_shared`, `pg_try_advisory_xact_lock_shared`, `pg_advisory_lock_shared`, `pg_try_advisory_lock_shared` and `pg_advisory_unlock_shared` (unit SQL tests)
- [x] a string key hashes like the exclusive helpers: `hashtext` by default, `hashtextextended(key, seed)` with `Hash64(seed)`
- [x] nil transaction and unpinned or nil DB fail with `ErrValidation`, other dialects with `ErrUnsupported`, and no statement runs
- [x] on PostgreSQL, two shared holders coexist, a shared lock excludes the exclusive lock and the reverse, `Hash64(52)` meets a raw `hashtextextended(key, 52)` lock, and `Unlock` does not release a shared lock (integration tests)
- [x] `docs/ai-guide.md`, `docs/capabilities.md` and `docs/upgrading.md` (0.8.0) describe the helpers
- [x] a regression test pins the fix: red before the change, green after

## Evidence

- Red: ci-agent run 42978437547 on 6c459f1 (tests only) failed to compile: `advisory/advisory_shared_test.go:25:22: undefined: advisory.XactLockShared`, and `undefined: advisory.TryXactLockShared`, `advisory.LockShared`, `advisory.UnlockShared`, `advisory.LockSharedString` in `advisory/advisory_shared_integration_test.go`.
- Green: ci-agent run 43006451426, headSha 34744ea0c4f9c460e5df530799a019229de80e64 (after merging sprint-9; earlier green 42993926849 on 1ba675a), all jobs passed (unit, integration postgres, integration-mysql x3, lint, fuzz-smoke, bench-module, apidiff-docs).
- Unit SQL, hashing, refusal and ErrUnsupported criteria: `advisory/advisory_shared_test.go`. PostgreSQL behaviour criteria: `advisory/advisory_shared_integration_test.go`. An earlier run (42984986678) hung on a test that opened four transactions against a three-connection pool; fixed in 4c13e91.
- Docs changed: `docs/ai-guide.md`, `docs/capabilities.md`, `docs/upgrading.md` (Upgrading to 0.8.0), `docs/reference.md` (from the CI artifact).
