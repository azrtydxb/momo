# momo-09 Task 4: No hidden goroutines and no OpenTelemetry in the core

Status: done 2026-11-10
Created: 2026-11-04
Epic: momo-09-observability-testing
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

Implements plan momo-09-observability-testing Task 4: No hidden goroutines and no OpenTelemetry in the core (`.procoder/plans/momo-09-observability-testing.md:748`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-18, S-35. Files: `obs_isolation_test.go`. Contributes to S-18, S-35; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestNoHiddenGoroutines` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestCoreHasNoOTel` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestOtelImportsDetector` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Built: db_obs_isolation_test.go (TestOtelImportsDetector, TestCoreHasNoOTel, unit) and db_obs_isolation_integration_test.go (TestNoHiddenGoroutines, both dialects). Green: ci-agent run 42778091550 (headSha 79a4a965871045b635c90908822154c8c19c4503, all 9 jobs success).
- Red proof was by mutation, not before the implementation (the tests guard an invariant that already held): ci-agent run 42778360116 on throwaway branch worktree-agent-a789ccf8e0cfe8888-mut (c1d5189, deliberate breaks) adds `go func() { select {} }()` to `momo.New`; TestNoHiddenGoroutines fails on postgres and sqlite. The OTel mutation (plan step 5) and the detector mutation (step 2) were not run; TestOtelImportsDetector asserts the detector result directly.

- Review fixes: TestCoreHasNoOTel now runs `go list -deps ./...` over the whole root module; TestNoHiddenGoroutines also runs the slow-query hook with plan capture, in and out of a transaction.
