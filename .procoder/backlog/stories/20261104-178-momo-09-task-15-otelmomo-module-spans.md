# momo-09 Task 15: otelmomo module, spans

Status: done 2026-11-10
Created: 2026-11-04
Epic: momo-09-observability-testing
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

Implements plan momo-09-observability-testing Task 15: otelmomo module, spans (`.procoder/plans/momo-09-observability-testing.md:2967`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-35. Files: `otelmomo/go.mod`, `otelmomo/doc.go`, `otelmomo/system_internal_test.go`.

## Acceptance criteria

- [x] `TestSystemName` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestOtelSpans` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestNewRejectsNilProviders` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-35] `TestOtelSpans` (otelmomo module) asserts span names, `db.system.name` (current OpenTelemetry database semantic conventions), `db.query.text` without parameter values, and error status via an in-memory exporter; `go list -deps` on the core module shows no OpenTelemetry package; fails if the core imports OTel.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Built: new nested module otelmomo/ (go.mod, go.sum taken from a CI `go mod tidy` run, doc.go, system.go, otelmomo.go, tests), wired into Makefile test, vet, tidy-check, staticcheck, govulncheck, test-integration-postgres, and dependabot.yml. Green: ci-agent run 42778091550 (headSha 79a4a965871045b635c90908822154c8c19c4503, all 9 jobs success): `unit` runs the otelmomo tests (TestSystemName, TestOtelSpans, TestNewRejectsNilProviders) on SQLite, `integration (postgres)` on PostgreSQL too; `go list -deps` guard is TestCoreHasNoOTel (story 168).
- Red proof by mutation was not run for the parameter-leak mutation; TestOtelSpans asserts no attribute value contains the argument. Docs: docs/observability.md "OpenTelemetry".
