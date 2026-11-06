# diff marks a whole file no-transaction when it has a concurrent index

Status: done 2026-11-06
Created: 2026-11-06
Epic: momo-04-migrations-core-postgresql
Sprint: 003-sprint-3-postgresql-hardening-migration-safety-bugs-query
Type: bug
Severity: s3

## Description

In migrate/engine.go ~719 diff writes `-- momo:no-transaction` for the whole file as soon as one statement is a CREATE INDEX CONCURRENTLY. CREATE TABLE, ADD COLUMN and other DDL in the same file then run outside a transaction; a failure midway leaves a half-applied migration that cannot be rolled back automatically.

Expected: diff splits concurrent indexes into a separate trailing migration file so the transactional DDL stays atomic.

## Acceptance criteria

- [x] a diff mixing CREATE TABLE and a concurrent index yields two files; the first is transactional, the second carries no-transaction
- [x] a diff with only concurrent indexes yields one no-transaction file
- [x] apply of the pair works and a failure in the first file leaves no partial DDL
- [x] a regression test pins the fix: red before the change, green after

## Evidence

- TestDiffSplitsConcurrentIndexFromTransactionalDDL: CREATE TABLE plus concurrent index gives two up files, head without the directive, tail with it; ValidateOffline accepts the pair.
- TestDiffConcurrentlyReachesRenderer: only a concurrent index gives one no-transaction file and Next is nil.
- TestSplitPairAppliesLockFree and TestSplitHeadFailureLeavesNoPartialDDL (integration, migrate/diff_lockfree_integration_test.go): CI run 42300960819 (commit 841c1cb) passed all four integration jobs (Go 1.26 and 1.27, PostgreSQL 16 and 17), which run the integration-tagged tests.
- TestDiffSplitsConcurrentIndexFromTransactionalDDL and TestDiffNoTransactionHeader: red before the change (one mixed file), green after.
