# momo-04 Task 8: Pure diff engine with explicit renames, destructive gating, staged NOT NULL, FK actions

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: -

## Description

Implements plan momo-04-migrations Task 8: Pure diff engine with explicit renames, destructive gating, staged NOT NULL, FK actions (`.procoder/plans/momo-04-migrations.md:934`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-10, S-11, S-74. Files: `migrate/diff.go`.

## Acceptance criteria

- [ ] `TestNoRenameGuessing` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestDestructiveRequiresFlag` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestStagedChangeDiff` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestFKActions` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestDiffNoChangeEmpty` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-10] `TestCLIContracts`: each command's exit codes and outputs as documented in Interfaces; offline commands run with no database URL set; fails if an offline command opens a connection.
- [ ] [S-11] `TestStagedChange` on a populated DB: add nullable column → data migration backfill → SET NOT NULL applies and all rows satisfy the constraint; fails if diff emits NOT NULL directly for a populated table without a default.
- [ ] [S-74] `TestFKActions` asserts each action's effect on every dialect and that the no-constraint option produces no FK in inspection; fails if actions are lost in a round trip.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

