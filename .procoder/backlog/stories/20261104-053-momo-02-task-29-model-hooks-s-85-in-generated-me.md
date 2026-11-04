# momo-02 Task 29: Model hooks (S-85) in generated metadata

Status: open
Created: 2026-11-04
Epic: momo-02-generator
Sprint: -

## Description

Implements plan momo-02-generator Task 29: Model hooks (S-85) in generated metadata (`.procoder/plans/momo-02-generator.md:3343`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-85. Files: `gen/hooks.go`, `gen/hooks_test.go`, `gen/testdata/fixtures/hooks/hooks.go`, `gen/testdata/fixtures/hooksbad/bad.go`, `gen/testdata/golden/hooks/momo_gen.go`, `gen/emit_meta.go`.

## Acceptance criteria

- [ ] `TestModelHookEmission` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestZeroHookSet` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestModelHookSignature` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-85] `TestModelHooks` asserts invocation order, error abort before SQL, and zero reflection in the call path (benchmark allocation check); fails if a hook runs for a builder that did not opt in.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

