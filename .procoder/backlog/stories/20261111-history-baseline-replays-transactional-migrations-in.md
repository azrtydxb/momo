# history Baseline replays transactional migrations in autocommit

Status: done 2026-11-11
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

- [x] a regression test pins the fix: red before the change, green after
- [x] a migration that is not NoTx runs between BEGIN and COMMIT, and a failing statement rolls it back and returns the error
- [x] a NoTx migration still runs in autocommit (CREATE INDEX CONCURRENTLY works)
- [x] docs/importers.md and the 0.8.0 section of docs/upgrading.md describe it

## Evidence

- Red: ci-agent run 42978022026 on throwaway branch worktree-agent-red-87-88 (commit 1268ab8, branch deleted), integration (postgres):
  TestBaselineRunsATransactionalMigrationInOneTransaction failed with
  `history: 00001_x.sql statement 3 on the scratch database: ERROR: relation "m" does not exist (SQLSTATE 42P01)`;
  TestBaselineRollsBackAndHonoursNoTx failed with `the failed migration must be rolled back, but table a exists`.
- Green: ci-agent run 42978042350 on 76e9afd, all 9 jobs green (fix commit 483fdd8 is in that history).
- NoTx still autocommit: TestBaselineRollsBackAndHonoursNoTx runs CREATE INDEX CONCURRENTLY in a NoTx migration, green.
- Docs: docs/importers.md, docs/migrations.md, docs/upgrading.md (0.8.0). No docs/reference.md change: the regenerated-docs job produced no artifact, so there was no diff.

## Evidence, review round (failed COMMIT, SQLite)

- Red: test-only commit 00d230d on a throwaway branch (deleted), ci-agent run
  42985357358: TestBaselineRollsBackAFailedCommitOnSQLite failed with
  `the next Baseline after a failed commit: history: the scratch database is not empty`
  (the failed COMMIT left the transaction open on the pooled connection).
- Fix: runMigration has one rollback helper shared by the statement-failure
  path, the failed-COMMIT path and the deferred cleanup (before DISCARD ALL).
- Docs: docs/importers.md and docs/upgrading.md (0.8.0) say rollback holds on
  PostgreSQL and SQLite, and MySQL commits DDL implicitly.
- Green: (pending)
