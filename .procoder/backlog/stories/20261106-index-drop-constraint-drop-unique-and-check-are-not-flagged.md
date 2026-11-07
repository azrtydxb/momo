# Index drop, constraint drop, UNIQUE and CHECK are not flagged risky or emitted safely

Status: done 2026-11-06
Created: 2026-11-06
Epic: momo-04-migrations-core-postgresql
Sprint: 003-sprint-3-postgresql-hardening-migration-safety-bugs-query
Type: bug
Severity: s3

## Description

Dropping an index or constraint and adding UNIQUE or CHECK constraints are not flagged as risky by diff. CHECK is not emitted `NOT VALID` (followed by VALIDATE CONSTRAINT), and UNIQUE is not emitted as `CREATE UNIQUE INDEX CONCURRENTLY` plus `ADD CONSTRAINT ... USING INDEX`. On a large table these take ACCESS EXCLUSIVE or long locks and full scans under the apply lock.

Expected: risky classification for these operations and lock-safe SQL for CHECK and UNIQUE.

## Acceptance criteria

- [x] diff reports drop index, drop constraint, add UNIQUE and add CHECK as risky
- [x] ADD CHECK is emitted NOT VALID with a separate VALIDATE CONSTRAINT
- [x] ADD UNIQUE is emitted via CREATE UNIQUE INDEX CONCURRENTLY and USING INDEX, in a no-transaction file
- [x] a regression test pins the fix: red before the change, green after

## Evidence

- TestDiffReportsRiskyChanges: risky hints and Change.Risky for add_unique, add_check and drop_index; Destructive stays false; fails when Risky marking is removed.
- TestDiffUniqueAndCheckAreLockSafe: head has ADD CONSTRAINT ... CHECK ... NOT VALID, tail has VALIDATE CONSTRAINT; fails without NOT VALID.
- TestDiffUniqueAndCheckAreLockSafe: tail (no-transaction) has CREATE UNIQUE INDEX CONCURRENTLY and ADD CONSTRAINT ... UNIQUE USING INDEX; fails without USING INDEX.
- TestDiffReportsRiskyChanges and TestDiffUniqueAndCheckAreLockSafe: red before the change, green after.
