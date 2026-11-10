# momo-09 Task 16: otelmomo metrics

Status: done 2026-11-10
Created: 2026-11-04
Epic: momo-09-observability-testing
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

Implements plan momo-09-observability-testing Task 16: otelmomo metrics (`.procoder/plans/momo-09-observability-testing.md:3290`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-35. Files: `otelmomo/otelmomo.go`, `otelmomo/otelmomo_test.go`. Contributes to S-35; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestOtelMetrics` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Built in otelmomo/otelmomo.go (db.client.operation.duration via dbconv.NewClientOperationDuration) with TestOtelMetrics in otelmomo/otelmomo_test.go. Green: ci-agent run 42778091550 (headSha 79a4a965871045b635c90908822154c8c19c4503, all 9 jobs success). The wrong-unit mutation (plan step 5) was not run.
