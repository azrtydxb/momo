# Rename-candidate exception skips staging for NOT NULL columns

Status: open
Created: 2026-11-06
Epic: momo-04-migrations-core-postgresql
Sprint: 003-sprint-3-postgresql-hardening-migration-safety-bugs-query
Type: bug
Severity: s4

## Description

In migrate/diff.go ~478 a column that is a rename candidate bypasses the NOT NULL staging refusal. With --allow-destructive the diff then emits an unstaged ADD COLUMN ... NOT NULL, which fails on a populated table or rewrites it.

Expected: the rename-candidate path still stages (or refuses) NOT NULL additions.

## Acceptance criteria

- [ ] diff on a rename candidate with a NOT NULL add and --allow-destructive emits a staged or refused add, never plain NOT NULL on an existing table
- [ ] a test pins the rule
- [ ] a regression test pins the fix: red before the change, green after

## Evidence

<!-- Filled at close time: the commands run and what their output proved,
     one line per criterion. Empty evidence keeps the story open. -->

