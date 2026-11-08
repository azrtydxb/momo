# momo-03 Task 13: Pool statistics track acquisition (S-90)

Status: done 2026-11-08
Created: 2026-11-04
Epic: momo-03-drivers-postgresql
Sprint: -

## Description

Implements plan momo-03-drivers Task 13: Pool statistics track acquisition (S-90) (`.procoder/plans/momo-03-drivers.md:1581`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-56, S-90. Files: `driver/pgxdriver/pin_test.go`. Contributes to S-56, S-90; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestPoolStatsReflectAcquisition` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

CI run 42457640580 on b7ec703 (ci-agent: unit, lint, fuzz-smoke, integration postgres) concluded success; the named tests passed there.

Red evidence: ci-agent run 42453837637 on throwaway branch worktree-agent-ac8514dccee058855-red (commit a900207, the code under test broken, tests kept, branch since deleted) failed: --- FAIL: TestPoolStatsReflectAcquisition, with PoolStats reporting AcquireCount 0.
