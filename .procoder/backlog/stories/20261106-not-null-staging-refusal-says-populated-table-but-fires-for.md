# NOT NULL staging refusal says populated table but fires for any existing table

Status: open
Created: 2026-11-06
Epic: momo-04-migrations-core-postgresql
Sprint: 003-sprint-3-postgresql-hardening-migration-safety-bugs-query
Type: bug
Severity: s4

## Description

The refusal in migrate/diff.go 432 and migrate/errors.go 21 says the table is populated, but the check only knows the table exists in the previous snapshot; it never queries for rows. Users with an empty table are told something false.

Expected: wording says existing table, or the check queries for rows.

## Acceptance criteria

- [ ] the message does not claim the table is populated unless the check verified it
- [ ] a test pins the message for an existing empty table
- [ ] a regression test pins the fix: red before the change, green after

## Evidence

<!-- Filled at close time: the commands run and what their output proved,
     one line per criterion. Empty evidence keeps the story open. -->

