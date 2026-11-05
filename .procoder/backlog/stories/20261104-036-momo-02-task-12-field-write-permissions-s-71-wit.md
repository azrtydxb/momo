# momo-02 Task 12: Field write permissions (S-71) with compile-fail fixtures

Status: open
Created: 2026-11-04
Epic: momo-02-generator
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-02-generator Task 12: Field write permissions (S-71) with compile-fail fixtures (`.procoder/plans/momo-02-generator.md:1071`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-71. Files: `momo/permissions.go`, `momo/permissions_test.go`, `gen/permissions.go`, `gen/permissions_test.go`, `gen/testdata/permissions/model/item.go`, `gen/testdata/permissions/item/momo_gen.go` ….

## Acceptance criteria

- [ ] `TestPermissionTypesMirrorColumn` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestFieldPermissions` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-71] `TestFieldPermissions` plus compile-fail fixtures assert Set on a read-only/immutable column does not compile and create-only columns are rejected in updates; fails if any path writes them.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

