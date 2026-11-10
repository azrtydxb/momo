# momo-09 Task 15: otelmomo module, spans

Status: open
Created: 2026-11-04
Epic: momo-09-observability-testing
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

Implements plan momo-09-observability-testing Task 15: otelmomo module, spans (`.procoder/plans/momo-09-observability-testing.md:2967`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-35. Files: `otelmomo/go.mod`, `otelmomo/doc.go`, `otelmomo/system_internal_test.go`.

## Acceptance criteria

- [ ] `TestSystemName` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestOtelSpans` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestNewRejectsNilProviders` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-35] `TestOtelSpans` (otelmomo module) asserts span names, `db.system.name` (current OpenTelemetry database semantic conventions), `db.query.text` without parameter values, and error status via an in-memory exporter; `go list -deps` on the core module shows no OpenTelemetry package; fails if the core imports OTel.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

