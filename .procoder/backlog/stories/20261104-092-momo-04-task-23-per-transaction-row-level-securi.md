# momo-04 Task 23: Per-transaction row-level-security scope helper

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: -

## Description

Implements plan momo-04-migrations Task 23: Per-transaction row-level-security scope helper (`.procoder/plans/momo-04-migrations.md:2566`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-38. Files: `tx_scope.go`.

## Acceptance criteria

- [ ] `TestRowLevelSecurity` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-38] `TestRowLevelSecurity`: with a policy applied by migration, a transaction scoped to tenant A cannot read tenant B's rows even through raw SQL, and the setting does not persist on the pooled connection; fails if `set_config` is session-scoped.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

