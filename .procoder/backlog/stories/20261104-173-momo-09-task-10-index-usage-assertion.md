# momo-09 Task 10: Index-usage assertion

Status: done 2026-11-10
Created: 2026-11-04
Epic: momo-09-observability-testing
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

Implements plan momo-09-observability-testing Task 10: Index-usage assertion (`.procoder/plans/momo-09-observability-testing.md:1775`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-55. Files: `momotest/index.go`, `momotest/index_internal_test.go`, `momotest/index_test.go`.

## Acceptance criteria

- [x] `TestPlanUsesIndex` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestAssertUsesIndex` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-55] `TestAssertUsesIndex` passes for an indexed lookup and fails for a sequential scan; fails if the helper ignores the plan.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Built: momotest/index.go (`AssertUsesIndex`, `planUsesIndex`), index_internal_test.go (TestPlanUsesIndex), index_test.go (TestAssertUsesIndex with scripted plans through the fake driver), index_integration_test.go (real PostgreSQL with and without the index; real SQLite). Package `momotest` internal test files use `momotest` as package name (the integration-tag convention test forbids untagged DB-opening files). Green: ci-agent run 42778091550 (headSha 79a4a965871045b635c90908822154c8c19c4503, all 9 jobs success).
- Red proof by mutation: ci-agent run 42778360116 on throwaway branch worktree-agent-a789ccf8e0cfe8888-mut (c1d5189, deliberate breaks) makes the plan matcher match every line; TestPlanUsesIndex, TestAssertUsesIndex, TestAssertUsesIndexPostgres and TestAssertUsesIndexSQLite fail.
- Docs: docs/testing.md section "Asserting index use".
- Review fix: index names match quoted (`"MyIdx"`) and end at whitespace or end of line, not at a `\b` (so `idx-2` and `idx.old` do not match `idx`); cases added to TestPlanUsesIndex and a mixed-case index on real PostgreSQL.
