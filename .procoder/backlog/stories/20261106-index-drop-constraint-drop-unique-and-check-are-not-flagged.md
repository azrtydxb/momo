# Index drop, constraint drop, UNIQUE and CHECK are not flagged risky or emitted safely

Status: open
Created: 2026-11-06
Epic: momo-04-migrations-core-postgresql
Sprint: -
Type: bug
Severity: s3

## Description

Dropping an index or constraint and adding UNIQUE or CHECK constraints are not flagged as risky by diff. CHECK is not emitted `NOT VALID` (followed by VALIDATE CONSTRAINT), and UNIQUE is not emitted as `CREATE UNIQUE INDEX CONCURRENTLY` plus `ADD CONSTRAINT ... USING INDEX`. On a large table these take ACCESS EXCLUSIVE or long locks and full scans under the apply lock.

Expected: risky classification for these operations and lock-safe SQL for CHECK and UNIQUE.

## Acceptance criteria

- [ ] diff reports drop index, drop constraint, add UNIQUE and add CHECK as risky
- [ ] ADD CHECK is emitted NOT VALID with a separate VALIDATE CONSTRAINT
- [ ] ADD UNIQUE is emitted via CREATE UNIQUE INDEX CONCURRENTLY and USING INDEX, in a no-transaction file
- [ ] a regression test pins the fix: red before the change, green after

## Evidence

<!-- Filled at close time: the commands run and what their output proved,
     one line per criterion. Empty evidence keeps the story open. -->

