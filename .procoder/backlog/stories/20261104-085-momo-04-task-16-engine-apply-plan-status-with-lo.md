# momo-04 Task 16: Engine Apply, Plan, Status with lock, checksum and dirty gating

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: -

## Description

Implements plan momo-04-migrations Task 16: Engine Apply, Plan, Status with lock, checksum and dirty gating (`.procoder/plans/momo-04-migrations.md:1903`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-10, S-11. Files: `migrate/engine.go`, `migrate/engine_apply_test.go`.

## Acceptance criteria

- [ ] `TestConcurrentApply` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestApplyTo` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestChecksumMismatchRefusesApply` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-10] `TestCLIContracts`: each command's exit codes and outputs as documented in Interfaces; offline commands run with no database URL set; fails if an offline command opens a connection.
- [ ] [S-11] `TestStagedChange` on a populated DB: add nullable column → data migration backfill → SET NOT NULL applies and all rows satisfy the constraint; fails if diff emits NOT NULL directly for a populated table without a default.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

