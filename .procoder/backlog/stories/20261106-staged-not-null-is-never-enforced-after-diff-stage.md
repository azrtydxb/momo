# Staged NOT NULL is never enforced after diff --stage

Status: done 2026-11-06
Created: 2026-11-06
Epic: momo-04-migrations-core-postgresql
Sprint: 003-sprint-3-postgresql-hardening-migration-safety-bugs-query
Type: bug
Severity: s2

## Description

`momo migrate diff --stage` for ADD COLUMN NOT NULL writes the column nullable in the migration but records NOT NULL in the snapshot (migrate/engine.go ~660). Reproduce: add a NOT NULL field to a model on an existing table, run `diff --stage`, apply, backfill, run `diff` again. It prints no changes because snapshot and model agree, so `SET NOT NULL` is never emitted and the database keeps a nullable column forever. The same applies to an existing column made NOT NULL under --stage (only a hint is emitted). The hint text tells the user to re-run `migrate diff`, which is wrong.

Expected: the staged nullability is either recorded in the snapshot as nullable (so the next diff emits SET NOT NULL) or the next diff otherwise emits SET NOT NULL once staging is done; the hint matches real behaviour.

## Acceptance criteria

- [x] after `diff --stage` plus apply, a second `diff` emits `ALTER COLUMN ... SET NOT NULL` (or equivalent) for the staged column
- [x] `validate` and `status` stay consistent with the database after the staged step and after the follow-up step
- [x] the hint printed by diff describes a command sequence that actually works
- [x] the same holds for an existing column made NOT NULL under --stage
- [x] a regression test pins the fix: red before the change, green after

## Evidence

- TestStagedAddColumnIsEnforcedByTheNextDiff: after Diff with Stage the snapshot has the column nullable; a second Diff with EnforceNotNull emits `"tenant" SET NOT NULL`, a plain second Diff is refused, a third Diff reports no changes.
- TestStagedAddColumnIsEnforcedByTheNextDiff: the staged snapshot records the column nullable (what the database has after applying) and the follow-up snapshot records NOT NULL, so snapshot and database agree after each step; validate and status read only snapshots, sums and history, which this does not change (no local database; CI covers the integration suite).
- TestStagedChangeDiff and TestStagedExistingColumnIsEnforcedByTheNextDiff: the hint now says to run migrate diff with --enforce-not-null, the command the tests run and that emits the SET NOT NULL.
- TestStagedExistingColumnIsEnforcedByTheNextDiff: an existing nullable column made NOT NULL is refused, --stage writes nothing, --enforce-not-null emits SET NOT NULL and records NOT NULL; TestEnforceNotNullDoesNotAuthorizeNewColumns pins that the flag does not bypass staging of adds.
- Mutation: with the stagedSnapshot call in Engine.Diff disabled, TestStagedAddColumnIsEnforcedByTheNextDiff fails (snapshot records NOT NULL); restored, it passes.
