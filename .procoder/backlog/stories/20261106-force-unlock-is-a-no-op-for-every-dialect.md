# --force-unlock is a no-op for every dialect

Status: done 2026-11-06
Created: 2026-11-06
Epic: momo-04-migrations-core-postgresql
Sprint: 003-sprint-3-postgresql-hardening-migration-safety-bugs-query
Type: bug
Severity: s4

## Description

The flag is declared in migrate/types.go 125 and wired in migrate.go 512, but no dialect acts on it. Users reaching for it during a stuck lock believe they forced a release.

Expected: remove the flag, or rename it and make it explicit that advisory locks are session scoped and release on disconnect.

## Acceptance criteria

- [x] the flag is removed or does what its name says for PostgreSQL
- [x] help text and docs/migrations.md match behaviour
- [x] a regression test pins the fix: red before the change, green after

## Evidence

- The flag and ApplyOptions.ForceUnlock are removed. TestMigrateForceUnlockIsRemoved: `apply --force-unlock` exits 2 as an unknown flag, and the usage text no longer lists it. The integration subtest in TestMigrateApplyLockWait is changed to the same assertion.
- Usage text (and docs/migrations.md, docs/ai-guide.md, docs/capabilities.md) now say the lock is a session advisory lock freed on disconnect, with the pg_terminate_backend route; TestMigrateForceUnlockIsRemoved checks the usage names it.
- Red first: the old binary accepted --force-unlock and exited 3 on a held lock.

