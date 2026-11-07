# No way to accept an edited applied migration

Status: done 2026-11-06
Created: 2026-11-06
Epic: momo-04-migrations-core-postgresql
Sprint: 003-sprint-3-postgresql-hardening-migration-safety-bugs-query
Type: bug
Severity: s3

## Description

After an applied migration file is edited, `migrate hash` rewrites momo.sum but the checksum in the history table stays, so apply stays blocked. The only route is a manual `UPDATE momo_migrations SET checksum = ...`.

Expected: a command such as `migrate rehash --version V --accept` that updates the history checksum for a named version after printing the diff, and records the action.

## Acceptance criteria

- [x] a rehash command with --version and --accept updates the history checksum for that version only
- [x] without --accept nothing changes and the command explains why
- [x] after rehash, status and apply no longer report a checksum mismatch
- [x] history rows of other versions are untouched
- [x] a regression test pins the fix: red before the change, green after

## Evidence

- TestRehash/accept..., TestRehashOnPostgres (CI), TestMigrateRehashAcceptsEditedMigration (CI): `migrate rehash --version V --accept` updates the checksum of V only; v2's row is compared before and after. The rehash action is recorded in the row's error column.
- TestRehash/without_accept_nothing_changes: no statement is executed and the row is unchanged; the CLI explains --accept and exits 1 (CLI test in CI); TestMigrateRehashUsage pins --version required.
- TestRehash/accept...: apply refuses with ErrChecksumMismatch before, succeeds after, and Status reports every version applied.
- Other versions untouched: TestRehash compares the v2 row before and after. The scoping WHERE clause is also exercised on real PostgreSQL by TestRehashOnPostgres in CI (the in-memory fake cannot catch a broken WHERE).
- Red first: Engine.Rehash and the rehash command did not exist.

