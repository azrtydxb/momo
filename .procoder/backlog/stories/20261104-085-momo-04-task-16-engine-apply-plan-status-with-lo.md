# momo-04 Task 16: Engine Apply, Plan, Status with lock, checksum and dirty gating

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: -

## Description

Note (Task 9 review): use `migrate.OrderErr` (or `OrderWithViews`), not `Order`, and surface the cycle error it returns.

Implements plan momo-04-migrations Task 16: Engine Apply, Plan, Status with lock, checksum and dirty gating (`.procoder/plans/momo-04-migrations.md:1903`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-10, S-11. Files: `migrate/engine.go`, `migrate/engine_apply_test.go`. Contributes to S-10, S-11; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestConcurrentApply` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestApplyTo` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestChecksumMismatchRefusesApply` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

- Deferred from story 082 (owner: this story, once the Engine and `migratetest` exist): (1) `TestPostgresObjects` run through the Engine (`migratetest.NewEngine`, `e.Diff`, `e.Apply`, `e.Inspect`) exactly as the plan's Task 13 Step 1 writes it, including the CONCURRENTLY index applied with `NoTx`; (2) the S-39 read-back for `TestManagedFunctionsTriggersPartitions`: no diff after apply and no function, trigger or table reported as unmanaged by `e.Drift`. The function and trigger read-back also needs story 081's inspection.
