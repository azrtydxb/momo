# momo-02 Task 29: Model hooks (S-85) in generated metadata

Status: open
Created: 2026-11-04
Epic: momo-02-generator
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

Implements plan momo-02-generator Task 29: Model hooks (S-85) in generated metadata (`.procoder/plans/momo-02-generator.md:3343`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-85. Files: `gen/hooks.go`, `gen/hooks_test.go`, `gen/testdata/fixtures/hooks/hooks.go`, `gen/testdata/fixtures/hooksbad/bad.go`, `gen/testdata/golden/hooks/momo_gen.go`, `gen/emit_meta.go`. Contributes to S-85; those criteria are verified in the stories that define their named tests. Scheduled after momo-05 Task 27: generated hooks populate `HookSet[T]`.

## Acceptance criteria

- [ ] `TestModelHookEmission` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestZeroHookSet` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestModelHookSignature` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

