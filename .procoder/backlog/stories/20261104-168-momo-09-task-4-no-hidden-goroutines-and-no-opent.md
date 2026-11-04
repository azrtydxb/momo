# momo-09 Task 4: No hidden goroutines and no OpenTelemetry in the core

Status: open
Created: 2026-11-04
Epic: momo-09-observability-testing
Sprint: -

## Description

Implements plan momo-09-observability-testing Task 4: No hidden goroutines and no OpenTelemetry in the core (`.procoder/plans/momo-09-observability-testing.md:748`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-18, S-35. Files: `obs_isolation_test.go`.

## Acceptance criteria

- [ ] `TestNoHiddenGoroutines` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestCoreHasNoOTel` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestOtelImportsDetector` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-18] `BenchmarkSelectNoHooks` allocations stay within the budget recorded in docs/PERFORMANCE.md; `TestHooks` asserts SQL, duration, rows and errors are reported and args only with WithArgLogging; `TestExplainAnalyzeExplicit` asserts plain Explain never executes the query; fails if disabled hooks allocate.
- [ ] [S-35] `TestOtelSpans` (otelmomo module) asserts span names, `db.system.name` (current OpenTelemetry database semantic conventions), `db.query.text` without parameter values, and error status via an in-memory exporter; `go list -deps` on the core module shows no OpenTelemetry package; fails if the core imports OTel.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

