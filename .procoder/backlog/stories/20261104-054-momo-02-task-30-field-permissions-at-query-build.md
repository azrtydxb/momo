# momo-02 Task 30: Field permissions at query build time (S-71, untyped paths)

Status: open
Created: 2026-11-04
Epic: momo-02-generator
Sprint: -

## Description

Implements plan momo-02-generator Task 30: Field permissions at query build time (S-71, untyped paths) (`.procoder/plans/momo-02-generator.md:3512`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-71. Files: `momo/permissions_runtime_test.go`, `momo/permissions.go`, `gen/testdata/permissions/model/momo_gen.go`. Contributes to S-71; those criteria are verified in the stories that define their named tests. Scheduled after momo-05: runtime permission checks use the Update/Insert builders.

## Acceptance criteria

- [ ] `TestFieldPermissionsRuntime` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

