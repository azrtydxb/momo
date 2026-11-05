# momo-02 Task 12: Field write permissions (S-71) with compile-fail fixtures

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-02-generator
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-02-generator Task 12: Field write permissions (S-71) with compile-fail fixtures (`.procoder/plans/momo-02-generator.md:1071`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-71. Files: `momo/permissions.go`, `momo/permissions_test.go`, `gen/permissions.go`, `gen/permissions_test.go`, `gen/testdata/permissions/model/item.go`, `gen/testdata/permissions/item/momo_gen.go` ….

## Acceptance criteria

- [x] `TestPermissionTypesMirrorColumn` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestFieldPermissions` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-71] `TestFieldPermissions` plus compile-fail fixtures assert Set on a read-only/immutable column does not compile and create-only columns are rejected in updates; fails if any path writes them.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Commit af45a4a. Files: permissions.go, permissions_test.go, gen/permissions.go, gen/permissions_test.go, gen/embed.go, gen/testdata/permissions/.
- FAIL before: `undefined: ReadOnlyColumn` (root test); `valid fixture must build ... undefined: momo.NewReadOnlyColumn` (gen test). PASS after: both ok.
- Mutation: adding `Set(v T) Assignment` to ReadOnlyColumn made TestFieldPermissions fail with `readonly_set must not compile`; reverted.
- Gate: `go build`, `go vet`, `go test -race -count=1 ./...`, `make test-compilefail` pass; procoder check: 0 blocking findings.
