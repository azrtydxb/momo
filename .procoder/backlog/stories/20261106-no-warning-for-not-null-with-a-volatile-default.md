# No warning for NOT NULL with a volatile default

Status: open
Created: 2026-11-06
Epic: momo-04-migrations-core-postgresql
Sprint: 003-sprint-3-postgresql-hardening-migration-safety-bugs-query
Type: bug
Severity: s4

## Description

Adding a column with NOT NULL and a volatile default (for example random(), clock_timestamp(), gen_random_uuid()) rewrites the table under ACCESS EXCLUSIVE lock. diff gives no risk warning for it.

Expected: diff flags volatile defaults on ADD COLUMN as a table rewrite risk.

## Acceptance criteria

- [ ] diff warns for ADD COLUMN with a volatile default on an existing table
- [ ] constant defaults produce no warning
- [ ] a regression test pins the fix: red before the change, green after

## Evidence

<!-- Filled at close time: the commands run and what their output proved,
     one line per criterion. Empty evidence keeps the story open. -->

