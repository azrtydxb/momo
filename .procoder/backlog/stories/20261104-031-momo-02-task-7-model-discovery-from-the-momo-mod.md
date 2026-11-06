# momo-02 Task 7: Model discovery from the momo.Model marker

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-02-generator
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-02-generator Task 7: Model discovery from the momo.Model marker (`.procoder/plans/momo-02-generator.md:566`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-2, S-3. Files: `gen/model.go`, `gen/model_test.go`, `gen/testdata/fixtures/models/models.go`, `gen/load.go`, `gen/types.go`. Contributes to S-2, S-3; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestModelDiscovery` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- RED: `go test ./gen -run TestModelDiscovery$` failed with `models=[]` before gen/model.go existed.
- GREEN: the same test passes (commit ade3b89), as do TestModelDiscoveryNoModels and TestModelDiscoveryDiagnostics.
- `go build ./... && go vet ./... && go test -race -count=1 ./...` and `go test ./internal/repocheck -run TestImportGraph` pass.
- `procoder check` reports 0 blocking findings.

