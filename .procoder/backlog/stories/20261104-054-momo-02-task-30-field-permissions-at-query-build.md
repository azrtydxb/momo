# momo-02 Task 30: Field permissions at query build time (S-71, untyped paths)

Status: open
Created: 2026-11-04
Epic: momo-02-generator
Sprint: -

## Description

Implements plan momo-02-generator Task 30: Field permissions at query build time (S-71, untyped paths) (`.procoder/plans/momo-02-generator.md:3512`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-71. Files: `momo/permissions_runtime_test.go`, `momo/permissions.go`, `gen/testdata/permissions/model/momo_gen.go`.

## Acceptance criteria

- [ ] `TestFieldPermissionsRuntime` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-71] `TestFieldPermissions` plus compile-fail fixtures assert Set on a read-only/immutable column does not compile and create-only columns are rejected in updates; fails if any path writes them.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

