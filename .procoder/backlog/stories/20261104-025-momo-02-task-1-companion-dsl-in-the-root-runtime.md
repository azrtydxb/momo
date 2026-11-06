# momo-02 Task 1: Companion DSL in the root runtime and the golden helper

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-02-generator
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-02-generator Task 1: Companion DSL in the root runtime and the golden helper (`.procoder/plans/momo-02-generator.md:45`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-2. Files: `momo/companion.go`, `momo/companion_test.go`, `internal/golden/golden.go`, `internal/golden/golden_test.go`. Contributes to S-2; those criteria are verified in the stories that define their named tests. Depends on momo-04 Task 1 (story 070): the seven managed-object companion methods return schema types that task adds.

## Acceptance criteria

- [x] `TestCompanionDSLCompiles` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestGoldenDiff` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- `go test . -run TestCompanionDSLCompiles` failed first with `undefined: Config`; `go test ./internal/golden -run TestGoldenDiff` failed first with `undefined: Diff`. After `companion.go` and `golden.go` (commit 9bdcece) both pass.
- `go build ./...`, `go vet ./...`, `go test -race -count=1 ./...` pass; `go test ./internal/repocheck -run TestImportGraph` passes; `procoder check` reports 0 blocking findings.
- The seven managed-object methods take the landed `schema.View`, `Function`, `Trigger`, `Policy`, `RLS`, `Partition`, `PartitionOf`; `TestCompanionManagedObjectsCompile` covers them.
