# No warning for NOT NULL with a volatile default

Status: done 2026-11-06
Created: 2026-11-06
Epic: momo-04-migrations-core-postgresql
Sprint: 003-sprint-3-postgresql-hardening-migration-safety-bugs-query
Type: bug
Severity: s4

## Description

Adding a column with NOT NULL and a volatile default (for example random(), clock_timestamp(), gen_random_uuid()) rewrites the table under ACCESS EXCLUSIVE lock. diff gives no risk warning for it.

Expected: diff flags volatile defaults on ADD COLUMN as a table rewrite risk.

## Acceptance criteria

- [x] diff warns for ADD COLUMN with a volatile default on an existing table
- [x] constant defaults produce no warning
- [x] a regression test pins the fix: red before the change, green after

## Evidence

- TestVolatileDefaultOnAddColumnWarns: random(), clock_timestamp(), gen_random_uuid() (any case or spacing), nested calls and nextval produce a risk hint naming ACCESS EXCLUSIVE; TestVolatileDefaultOnNewTableDoesNotWarn: a new table does not.
- TestConstantDefaultOnAddColumnDoesNotWarn: '', 0, now(), current_timestamp, a quoted 'random()' literal, coalesce(1, 2) and false produce no hint.
- Mutation: disabling volatileDefault makes TestVolatileDefaultOnAddColumnWarns fail.
