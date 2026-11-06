# lock_timeout and statement_timeout of 0s cannot disable the timeout

Status: open
Created: 2026-11-06
Epic: momo-04-migrations-core-postgresql
Sprint: -
Type: bug
Severity: s4

## Description

migrate/options.go 33-34 and migrate/postgres/session.go 58 treat a zero duration as unset and apply the default, so a user cannot disable lock_timeout or statement_timeout (PostgreSQL treats 0 as off), for example for a long index build.

Expected: an explicit 0s sets the PostgreSQL timeout to 0; only an absent value uses the default.

## Acceptance criteria

- [ ] momo.yaml with `statement_timeout: 0s` results in SET statement_timeout = 0 on the session
- [ ] an absent key still uses the default
- [ ] same for lock_timeout
- [ ] a regression test pins the fix: red before the change, green after

## Evidence

<!-- Filled at close time: the commands run and what their output proved,
     one line per criterion. Empty evidence keeps the story open. -->

