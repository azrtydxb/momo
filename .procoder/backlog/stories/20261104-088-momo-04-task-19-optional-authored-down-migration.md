# momo-04 Task 19: Optional authored down migrations and group rollback

Status: done 2026-11-08
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: -

## Description

Implements plan momo-04-migrations Task 19: Optional authored down migrations and group rollback (`.procoder/plans/momo-04-migrations.md:2194`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-11, S-94. Files: `migrate/rollback.go`. Contributes to S-11, S-94; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestRollbackGroup` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestRollbackNoDown` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

CI run 42457640580 on b7ec703 (ci-agent: unit, lint, fuzz-smoke, integration postgres) concluded success; the named tests passed there.

Red evidence: ci-agent run 42453062663 on throwaway branch worktree-agent-ac8514dccee058855-red (commit 6f002a6, implementation reverted, tests kept, branch since deleted) failed: migrate/rollback_test.go: package migrate FAIL [build failed]; undefined Engine.Rollback and RollbackOptions (TestRollbackGroup, TestRollbackNoDown).
