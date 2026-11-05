# momo-02 Task 14: MomoMeta emission in the model package

Status: open
Created: 2026-11-04
Epic: momo-02-generator
Sprint: -

## Description

Implements plan momo-02-generator Task 14: MomoMeta emission in the model package (`.procoder/plans/momo-02-generator.md:1444`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-3. Files: `gen/emit_meta.go`, `gen/emit_meta_test.go`, `gen/testdata/golden/meta/models/momo_gen.go`. Contributes to S-3; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestEmitMetaGolden` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

