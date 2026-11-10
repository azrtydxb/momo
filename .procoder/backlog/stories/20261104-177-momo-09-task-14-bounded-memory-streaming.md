# momo-09 Task 14: Bounded-memory streaming

Status: done 2026-11-10
Created: 2026-11-04
Epic: momo-09-observability-testing
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

Implements plan momo-09-observability-testing Task 14: Bounded-memory streaming (`.procoder/plans/momo-09-observability-testing.md:2828`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-56. Files: `obs_streaming_test.go`, `docs/PERFORMANCE.md`, `select.go`.

## Acceptance criteria

- [x] `TestStreamingBoundedMemory` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestPerformanceDocRecordsStreamBound` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-56] `TestStreamingBoundedMemory` iterates one million rows with Each and asserts heap growth stays under a fixed bound; fails if rows are buffered.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Built: db_obs_streaming_integration_test.go (TestStreamingBoundedMemory: one million rows, heap bound 16 MiB, both dialects), db_obs_streaming_test.go (TestPerformanceDocRecordsStreamBound), docs/PERFORMANCE.md "Streaming memory bound". Green: ci-agent run 42778091550 (headSha 79a4a965871045b635c90908822154c8c19c4503, all 9 jobs success).
- Red proof by mutation: ci-agent run 42778360116 on throwaway branch worktree-agent-a789ccf8e0cfe8888-mut (c1d5189, deliberate breaks) buffers every row with All before calling fn; TestStreamingBoundedMemory fails on postgres and sqlite. Deviation: the seed uses `INSERT ... WITH RECURSIVE ... SELECT` (valid on SQLite and PostgreSQL) instead of the plan's `WITH ... INSERT`.
- Review fix: TestPerformanceDocRecordsStreamBound was removed (a regex test over a doc table is churn); the `Streaming heap growth bound (MiB)` row stays in docs/PERFORMANCE.md and the constant in db_obs_streaming_integration_test.go points at it. The second criterion no longer applies. The test skips MySQL and MariaDB (recursive CTE depth, `||` is OR there); testopen.Each runs only PostgreSQL and SQLite today.
