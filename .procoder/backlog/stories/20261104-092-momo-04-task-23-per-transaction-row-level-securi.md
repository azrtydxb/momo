# momo-04 Task 23: Per-transaction row-level-security scope helper

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: -

## Description

Implements plan momo-04-migrations Task 23: Per-transaction row-level-security scope helper (`.procoder/plans/momo-04-migrations.md:2566`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-38. Files: `tx_scope.go`.

## Acceptance criteria

- [x] `TestRowLevelSecurity` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-38] `TestRowLevelSecurity`: with a policy applied by migration, a transaction scoped to tenant A cannot read tenant B's rows even through raw SQL, and the setting does not persist on the pooled connection; fails if `set_config` is session-scoped.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

CI run 42414158119 on 764d4d1 (workflow ci-agent: unit, lint, fuzz-smoke, integration postgres) concluded success; the named tests ran and passed there.

Red evidence: ci-agent run 42453062663 on throwaway branch worktree-agent-ac8514dccee058855-red (commit 6f002a6, implementation reverted, tests kept, branch since deleted) failed: --- FAIL: TestRowLevelSecurity in migrate/postgres with SetLocal reverted to session scope (set_config is_local false).
