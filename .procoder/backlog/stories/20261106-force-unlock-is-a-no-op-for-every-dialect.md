# --force-unlock is a no-op for every dialect

Status: open
Created: 2026-11-06
Epic: momo-04-migrations-core-postgresql
Sprint: -
Type: bug
Severity: s4

## Description

The flag is declared in migrate/types.go 125 and wired in migrate.go 512, but no dialect acts on it. Users reaching for it during a stuck lock believe they forced a release.

Expected: remove the flag, or rename it and make it explicit that advisory locks are session scoped and release on disconnect.

## Acceptance criteria

- [ ] the flag is removed or does what its name says for PostgreSQL
- [ ] help text and docs/migrations.md match behaviour
- [ ] a regression test pins the fix: red before the change, green after

## Evidence

<!-- Filled at close time: the commands run and what their output proved,
     one line per criterion. Empty evidence keeps the story open. -->

