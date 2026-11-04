# momo-01 Task 9: Fake driver and shared model fixtures

Status: done 2026-11-04
Created: 2026-11-04
Epic: momo-01-foundation
Sprint: 001-foundation-of-the-query-core-dialects-schema-base-types

## Description

Implements plan momo-01-foundation Task 9: Fake driver and shared model fixtures (`.procoder/plans/momo-01-foundation.md:885`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: supporting work for the plan. Files: `internal/fakedriver/fakedriver.go`, `unit_fixtures_test.go`.

## Acceptance criteria

- [x] `TestFakeDriverRecordsAndScripts` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Before the implementation: `go test ./internal/fakedriver/ -count=1` printed `internal/fakedriver/fakedriver_test.go:13:7: undefined: New` and `FAIL github.com/azrtydxb/momo/internal/fakedriver [build failed]`.
- After: `go test ./internal/fakedriver/ -count=1 -race` printed `ok  github.com/azrtydxb/momo/internal/fakedriver`.
- `go build ./... && go vet ./... && go test -race ./...` all pass.
- `procoder check`: `3 clean, 0 unformatted, 0 unchecked, 0 out of scope, 0 blocking`.
- Deviations (approved by the coordinator): the fixture Meta omits `CreatedAt`/`UpdatedAt` because the spike's `Meta` lacks them and Task 10 adds them (plan amended in 663c152). The compile-time `Modeler` assertion is dropped: a constraint interface cannot be a variable type, and `Select[unitDocument]` enforces it at each call site.
