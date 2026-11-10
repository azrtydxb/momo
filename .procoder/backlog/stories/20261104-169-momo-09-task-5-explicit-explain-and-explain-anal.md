# momo-09 Task 5: Explicit EXPLAIN and EXPLAIN ANALYZE helpers

Status: done 2026-11-10
Created: 2026-11-04
Epic: momo-09-observability-testing
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

Implements plan momo-09-observability-testing Task 5: Explicit EXPLAIN and EXPLAIN ANALYZE helpers (`.procoder/plans/momo-09-observability-testing.md:865`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-18. Files: `explain.go`, `obs_explain_test.go`.

## Acceptance criteria

- [x] `TestExplainAnalyzeExplicit` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-18] `BenchmarkSelectNoHooks` allocations stay within the budget recorded in docs/PERFORMANCE.md; `TestHooks` asserts SQL, duration, rows and errors are reported and args only with WithArgLogging; `TestExplainAnalyzeExplicit` asserts plain Explain never executes the query; fails if disabled hooks allocate.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Built: select_explainstmt.go (`momo.Explain`, `momo.ExplainAnalyze`; named to fit the root file-prefix rule), select_explainstmt_test.go (fake-driver unit tests per dialect, ExplainAnalyze rollback and unsupported-issues-nothing) and db_obs_explain_test.go (TestExplainAnalyzeExplicit, integration). Green: ci-agent run 42778091550 (headSha 79a4a965871045b635c90908822154c8c19c4503, all 9 jobs success).
- Red proof by mutation: ci-agent run 42778360116 on throwaway branch worktree-agent-a789ccf8e0cfe8888-mut (c1d5189, deliberate breaks) makes Explain send the bare statement; TestExplainAnalyzeExplicit and TestExplainStatementPerDialectFree fail.
- S-18: BenchmarkSelectNoHooks budget is story 167 (same green run); TestHooks (db_obs_hooks_test.go) already existed and runs in the integration job; TestExplainAnalyzeExplicit asserts Explain issues exactly one EXPLAIN statement and the INSERT is not executed. Docs: docs/observability.md (Explain), docs/ai-guide.md; docs/reference.md regenerated from CI output.
- Review fix: ExplainAnalyze now runs every statement, SELECT included, in a rolled-back transaction or savepoint; the doc names what a rollback cannot undo (nextval, NOTIFY, dblink). TestExplainAnalyzeExplicit checks that a SELECT calling a writing function leaves nothing behind.
