# Staged NOT NULL is never enforced after diff --stage

Status: open
Created: 2026-11-06
Epic: momo-04-migrations-core-postgresql
Sprint: -
Type: bug
Severity: s2

## Description

`momo migrate diff --stage` for ADD COLUMN NOT NULL writes the column nullable in the migration but records NOT NULL in the snapshot (migrate/engine.go ~660). Reproduce: add a NOT NULL field to a model on an existing table, run `diff --stage`, apply, backfill, run `diff` again. It prints no changes because snapshot and model agree, so `SET NOT NULL` is never emitted and the database keeps a nullable column forever. The same applies to an existing column made NOT NULL under --stage (only a hint is emitted). The hint text tells the user to re-run `migrate diff`, which is wrong.

Expected: the staged nullability is either recorded in the snapshot as nullable (so the next diff emits SET NOT NULL) or the next diff otherwise emits SET NOT NULL once staging is done; the hint matches real behaviour.

## Acceptance criteria

- [ ] after `diff --stage` plus apply, a second `diff` emits `ALTER COLUMN ... SET NOT NULL` (or equivalent) for the staged column
- [ ] `validate` and `status` stay consistent with the database after the staged step and after the follow-up step
- [ ] the hint printed by diff describes a command sequence that actually works
- [ ] the same holds for an existing column made NOT NULL under --stage
- [ ] a regression test pins the fix: red before the change, green after

## Evidence

<!-- Filled at close time: the commands run and what their output proved,
     one line per criterion. Empty evidence keeps the story open. -->

