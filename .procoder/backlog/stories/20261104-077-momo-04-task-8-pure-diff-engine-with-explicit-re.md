# momo-04 Task 8: Pure diff engine with explicit renames, destructive gating, staged NOT NULL, FK actions

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: -

## Description

Implements plan momo-04-migrations Task 8: Pure diff engine with explicit renames, destructive gating, staged NOT NULL, FK actions (`.procoder/plans/momo-04-migrations.md:934`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-10, S-11, S-74. Files: `migrate/diff.go`. Contributes to S-10, S-11; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestNoRenameGuessing` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestDestructiveRequiresFlag` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestStagedChangeDiff` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestFKActions` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestDiffNoChangeEmpty` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-74] `TestFKActions` asserts each action's effect on every dialect and that the no-constraint option produces no FK in inspection; fails if actions are lost in a round trip.
- [ ] `procoder check` gate is clean and the task's commit is made
- [ ] `Column.Collate` and `Column.Comment` are diffed: a change to either yields a column change (momo-02 Task 10 builds them from `collate` and `comment` tags and MomoConfig), with a round-trip test

## Evidence

