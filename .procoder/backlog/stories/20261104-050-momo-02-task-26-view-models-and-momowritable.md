# momo-02 Task 26: View models and MomoWritable

Status: open
Created: 2026-11-04
Epic: momo-02-generator
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

Implements plan momo-02-generator Task 26: View models and MomoWritable (`.procoder/plans/momo-02-generator.md:2748`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: supporting work for the plan. Files: `momo/view.go`, `momo/view_test.go`, `gen/view.go`, `gen/view_test.go`, `gen/testdata/views/model/doc.go`, `gen/testdata/views/model/momo_gen.go` …. Scheduled after momo-05: compile-fail fixtures use the Insert/Update/Delete builders.

## Acceptance criteria

- [ ] `TestViewModelsNotWritable` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

Needs decision: should momo add a compile-time rule that view models cannot be written?

Audit: nothing of this exists. There is no `momo.View` marker, no `WritableModeler`, no `MomoWritable` emission (`git grep` finds none outside plans and the maintainability audit, which lists `WritableModeler`/`MomoWritable` among the contract's unreferenced APIs). Writes are constrained by the same `Modeler[T]` as reads: `Insert` at insert.go:84, `Update` at update.go:46, `Delete` at delete.go:38, `SoftDelete` at meta_softdelete.go:77. Views are declared with the structured companion call `c.View(schema.View{...})` (meta_companion.go, read by gen/companion.go:48) and read through projections (docs/projections.md) or a model mapped to the view's name.

Options:

1. Build the plan as written: `momo.View` marker, `WritableModeler[T]` (`*T` plus `MomoWritable()`) as the constraint of `Insert`/`Update`/`Delete`/`SoftDelete`, generated `MomoWritable` for table models only. Strongest guarantee (a write to a view fails to compile), but it is an exported-API break: every hand-written or previously generated model must be regenerated or gain the method, so it needs a `feat!` and a release note.
2. Runtime guard only: a `view:` tag (or a `ReadOnly` flag on `Meta`) makes `Insert`/`Update`/`Delete` return `ErrUnsupported` when the statement is built. No API break, but the failure is at run time, not compile time.
3. Keep the shipped design (views via the companion `c.View` plus projections, no write guard) and close this story as not needed.

Nothing was built, so every criterion stays unticked. Stories 049 (plan gate names `TestViewModelsNotWritable`) and 051 (the "view model has no c.View" rule) depend on the answer.

