# advisory shared locks are missing (pg_advisory_xact_lock_shared and friends)

Status: open
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

- [ ] `advisory.XactLockShared`, `TryXactLockShared`, `LockShared`, `TryLockShared`, `UnlockShared` and their `...String` forms exist and render `pg_advisory_xact_lock_shared`, `pg_try_advisory_xact_lock_shared`, `pg_advisory_lock_shared`, `pg_try_advisory_lock_shared` and `pg_advisory_unlock_shared` (unit SQL tests)
- [ ] a string key hashes like the exclusive helpers: `hashtext` by default, `hashtextextended(key, seed)` with `Hash64(seed)`
- [ ] nil transaction and unpinned or nil DB fail with `ErrValidation`, other dialects with `ErrUnsupported`, and no statement runs
- [ ] on PostgreSQL, two shared holders coexist, a shared lock excludes the exclusive lock and the reverse, `Hash64(52)` meets a raw `hashtextextended(key, 52)` lock, and `Unlock` does not release a shared lock (integration tests)
- [ ] `docs/ai-guide.md`, `docs/capabilities.md` and `docs/upgrading.md` (0.8.0) describe the helpers
- [ ] a regression test pins the fix: red before the change, green after

## Evidence

<!-- Filled at close time. -->
