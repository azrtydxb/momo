# momo-09 Task 13: Slow-query hook with opt-in plain EXPLAIN

Status: done 2026-11-10
Created: 2026-11-04
Epic: momo-09-observability-testing
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

Implements plan momo-09-observability-testing Task 13: Slow-query hook with opt-in plain EXPLAIN (`.procoder/plans/momo-09-observability-testing.md:2409`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-54. Files: `slowquery.go`, `obs_slowquery_test.go`.

## Acceptance criteria

- [x] `TestSlowQueryHook` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-54] `TestSlowQueryHook` reports only queries over the threshold and captures a plain EXPLAIN plan only when enabled; fails if EXPLAIN ANALYZE ever runs.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Built as `obs.NewSlowQuery`, `obs.SlowQuery`, `obs.WithPlainExplain` (package obs, not the root: sprint 6 moved ready-made hooks into obs; the plan's root names `momo.NewSlowQueryHook` do not exist). obs/slowquery.go, obs/slowquery_integration_test.go (TestSlowQueryHook with the plan's subtests, adapted). Green: ci-agent run 42778091550 (headSha 79a4a965871045b635c90908822154c8c19c4503, all 9 jobs success) (the integration jobs run it on PostgreSQL, MySQL 8.4/9.7, MariaDB and SQLite).
- Deviation: a statement that binds a sensitive value is never explained (PlanSkipped says so), an addition for S-42.
- Plan mutations (no recursion guard, no savepoint, ANALYZE) were not run; the probe branch only ran the Explain, goroutine, streaming and index mutations.
- Docs: docs/observability.md "Slow queries", docs/ai-guide.md.
- Review fixes: a statement that failed with a cancellation or timeout gets no plan (obs/slowquery_test.go); inside a transaction the EXPLAIN runs without a deadline on the caller's connection (the 2 s deadline applies only outside a transaction); savepoint failures are classified (unsupported, context ended, tx finished, aborted, other); recursion-guard unit tests cover the in-tx and the other-querier paths. The integration test skips MySQL and MariaDB (recursive CTE depth).
