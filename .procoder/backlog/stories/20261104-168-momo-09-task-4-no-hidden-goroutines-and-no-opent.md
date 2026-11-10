# momo-09 Task 4: No hidden goroutines and no OpenTelemetry in the core

Status: open
Created: 2026-11-04
Epic: momo-09-observability-testing
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

Implements plan momo-09-observability-testing Task 4: No hidden goroutines and no OpenTelemetry in the core (`.procoder/plans/momo-09-observability-testing.md:748`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-18, S-35. Files: `obs_isolation_test.go`. Contributes to S-18, S-35; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestNoHiddenGoroutines` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestCoreHasNoOTel` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestOtelImportsDetector` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

