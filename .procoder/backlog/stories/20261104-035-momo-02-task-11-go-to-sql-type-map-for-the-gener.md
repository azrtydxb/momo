# momo-02 Task 11: Go-to-SQL type map for the generator

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-02-generator
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-02-generator Task 11: Go-to-SQL type map for the generator (`.procoder/plans/momo-02-generator.md:1008`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-1, S-3, S-31. Files: `gen/typemap/typemap.go`, `gen/typemap/typemap_test.go`, `gen/schema.go`. Contributes to S-1, S-3, S-31; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestGeneratorTypeMap` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

Red: `go test ./gen/typemap -count=1` failed with `undefined: Map` before implementation. Green: `ok github.com/azrtydxb/momo/gen/typemap`; `go build`, `go vet` and `go test -race -count=1 ./...` pass. Mutation check: mapping postgres int/int64 to `integer` made TestGeneratorTypeMap and TestGeneratorTypeMapFullTable fail; reverted. Gate (`procoder check`): 0 blocking findings. Commit d809973 `feat(gen): go to sql type map for generated schema`. Not done: the `gen/schema.go` call site, because that file is created by Task 10 (story 034).

