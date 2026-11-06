# diff marks a whole file no-transaction when it has a concurrent index

Status: open
Created: 2026-11-06
Epic: momo-04-migrations-core-postgresql
Sprint: -
Type: bug
Severity: s3

## Description

In migrate/engine.go ~719 diff writes `-- momo:no-transaction` for the whole file as soon as one statement is a CREATE INDEX CONCURRENTLY. CREATE TABLE, ADD COLUMN and other DDL in the same file then run outside a transaction; a failure midway leaves a half-applied migration that cannot be rolled back automatically.

Expected: diff splits concurrent indexes into a separate trailing migration file so the transactional DDL stays atomic.

## Acceptance criteria

- [ ] a diff mixing CREATE TABLE and a concurrent index yields two files; the first is transactional, the second carries no-transaction
- [ ] a diff with only concurrent indexes yields one no-transaction file
- [ ] apply of the pair works and a failure in the first file leaves no partial DDL
- [ ] a regression test pins the fix: red before the change, green after

## Evidence

<!-- Filled at close time: the commands run and what their output proved,
     one line per criterion. Empty evidence keeps the story open. -->

