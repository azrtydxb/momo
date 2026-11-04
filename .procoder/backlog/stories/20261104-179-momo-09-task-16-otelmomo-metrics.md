# momo-09 Task 16: otelmomo metrics

Status: open
Created: 2026-11-04
Epic: momo-09-observability-testing
Sprint: -

## Description

Implements plan momo-09-observability-testing Task 16: otelmomo metrics (`.procoder/plans/momo-09-observability-testing.md:3290`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-35. Files: `otelmomo/otelmomo.go`, `otelmomo/otelmomo_test.go`.

## Acceptance criteria

- [ ] `TestOtelMetrics` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-35] `TestOtelSpans` (otelmomo module) asserts span names, `db.system.name` (current OpenTelemetry database semantic conventions), `db.query.text` without parameter values, and error status via an in-memory exporter; `go list -deps` on the core module shows no OpenTelemetry package; fails if the core imports OTel.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

