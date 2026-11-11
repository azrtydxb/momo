# history Baseline replays transactional migrations in autocommit

Status: open
Created: 2026-11-11
Epic: -
Sprint: 009-fix-every-open-github-issue-86-to-108-the-kuvryn-orbit-port
Type: bug
Severity: s2

## Description

Issue #87 in azrtydxb/momo.

Reproduction: a goose migration without `-- +goose NO TRANSACTION` that runs
`CREATE TEMP TABLE m ON COMMIT DROP AS SELECT 1 AS id;` and then
`INSERT INTO t SELECT id FROM m;`. `goose.Read`, `history.Convert` and
`history.Baseline` fail with `relation "m" does not exist`.

Observed vs expected: `runMigration` in importer/history/history.go ran each
statement in autocommit on a pinned connection with no `BEGIN`, so an
`ON COMMIT DROP` table was dropped at once, `SET LOCAL` was lost and deferred
constraints were checked per statement. The source tools and `momo migrate
apply` run a transactional migration in one transaction, and so must the
replay. A NoTx migration keeps running in autocommit.

Root cause: no transaction around the statements of a migration.

## Acceptance criteria

- [ ] a regression test pins the fix: red before the change, green after
- [ ] a migration that is not NoTx runs between BEGIN and COMMIT, and a failing statement rolls it back and returns the error
- [ ] a NoTx migration still runs in autocommit (CREATE INDEX CONCURRENTLY works)
- [ ] docs/importers.md and the 0.8.0 section of docs/upgrading.md describe it

## Evidence

(pending CI)
