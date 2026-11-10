# momo-02 Task 29: Model hooks (S-85) in generated metadata

Status: done 2026-11-10
Created: 2026-11-04
Epic: momo-02-generator
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

Implements plan momo-02-generator Task 29: Model hooks (S-85) in generated metadata (`.procoder/plans/momo-02-generator.md:3343`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-85. Files: `gen/hooks.go`, `gen/hooks_test.go`, `gen/testdata/fixtures/hooks/hooks.go`, `gen/testdata/fixtures/hooksbad/bad.go`, `gen/testdata/golden/hooks/momo_gen.go`, `gen/emit_meta.go`. Contributes to S-85; those criteria are verified in the stories that define their named tests. Scheduled after momo-05 Task 27: generated hooks populate `HookSet[T]`.

## Acceptance criteria

- [x] `TestModelHookEmission` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestZeroHookSet` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestModelHookSignature` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

Built (gen/hooks.go `DetectHooks`, `Model.Hooks`, `Hooks:` line in gen/emit_meta.go, golden gen/testdata/golden/hooks/momo_gen.go taken from the CI log). CI: ci-agent run 42773371002, headSha 5acef91, all 9 jobs green: `TestModelHookEmission`, `TestZeroHookSet` and `TestModelHookSignature` (gen/hooks_test.go) pass.

Deviations from the plan text: the shipped `momo.HookSet` fields are `func(ctx, *T) error` (db_hooks.go), not `func(*T, context.Context)`, so the generator emits a closure per hook (`func(ctx context.Context, m *Doc) error { return m.BeforeInsert(ctx) }`) instead of a method expression, and imports `context` when a model has hooks. `DetectHooks` also takes the `*token.FileSet` for the diagnostic position.

Red evidence is not separate: tests and implementation were pushed together, and the first run (42769490088) failed only on the then-missing golden file. The "failed before the implementation" part of the three test criteria is therefore not shown, and they stay unticked.

### Sprint 7 close-out (2026-11-10)

Audit: the generator fills `Meta.Hooks` (gen/hooks.go, gen/emit_meta.go) and the runtime calls them: BeforeInsert/BeforeUpdate/BeforeDelete/AfterLoad (db_hooks.go, insert_copy.go), covered by TestModelHooks, TestModelHooksOnlyForOptedInBuilders, TestModelHooksBeforeAbortWithoutSQL, TestModelHooksAfterLoadOnEveryRead and TestModelHooksAfterLoadError. Nothing missing.

Red, shown retroactively on a throwaway branch: ci-agent run 42828247513, headSha afb8fa2 (`DetectHooks` returns nothing, `hooksLiteral` emits an empty `Hooks:` for hook-less models): `TestModelHookEmission`, `TestZeroHookSet` and `TestModelHookSignature` all FAIL. Green: ci-agent run 42838538032, headSha 5e36833, all 9 jobs. Gate: `procoder check` over the changed files, 27 clean, 0 blocking.
