# momo-02 Task 26: View models and a write guard

Status: done
Created: 2026-11-04
Epic: momo-02-generator
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

Plan momo-02-generator Task 26 (`.procoder/plans/momo-02-generator.md:2823`) asked for a compile-time `WritableModeler[T]` and `MomoWritable` rule. User decision 2026-11-10 (`.procoder/ask/decisions.md`, "Sprint 7 backlog decisions", view models, stories 050 and 101): build a runtime guard instead. Views are marked with `momo.View`, and the write builders return `ErrValidation` for a view model. No signature changes; the compile-time design is dropped.

## Acceptance criteria

- [x] A model embeds `momo.View` next to `momo.Model`; the generator reads the marker and records it in the generated `Meta` (`View: true`), and the marker is not a column
- [x] `Insert`, `Upsert`, `Update` (Set and Model), `SoftDelete`, `BulkUpdate`, `Delete` and `Merge` return `ErrValidation` naming the model when the model is a view; reads still work; no builder signature changes
- [x] `TestViewModelsNotWritable` (unit, one assertion per write path) and `TestViewMarkerEmission` (generator emission) exist and pass
- [x] `procoder check` gate is clean and the commits are made

## Evidence

CI: ci-agent run 42784895483, headSha d5d4696, all jobs green.

- `momo.View` marker: meta.go; `Meta.View` and `viewWriteErr`: meta.go. Generator: gen/model.go (marker discovery, `momo.View` by value, not a column; diagnostic on a projection), `Model.IsView` in gen/types.go, `View: true,` in gen/emit_meta.go.
- Guard in Insert/Upsert (insert.go), Update, SoftDelete and BulkUpdate (update.go plan, write_merge.go planBulk), Delete (delete.go), Merge (write_merge.go render). `TestViewModelsNotWritable` (meta_view_test.go) checks each path for `ErrValidation` and the model name, and that `Select` still builds.
- `TestViewMarkerEmission` (gen/view_test.go, fixture gen/testdata/fixtures/views/views.go): exactly one `View: true,`, on the view model only.
- Docs: docs/ai-guide.md "View models", docs/upgrading.md, docs/reference.md (generated, `momo.View`).
- Earlier run 42780991202 failed only on two docs checks (a compiled example block and the reference listing), fixed in d5d4696.
- Review fix (guard moved from the `Insert` constructor to `beforeWrite`, `plan`, `render` and `copyShape`): `TestViewModelsNotWritableThroughOtherInsertPaths` covers CreateGraph (view as parent and as child), FindOrCreate, OnConflict, Copy, Models.Exec, Upsert.Exec and SoftDelete.Exec. Green: ci-agent run 42793806622, headSha 791b2c6.
- Red evidence: throwaway branch worktree-agent-a2077a36ebbae03d4-red (947c06f = 4a9d64f plus only the new test), ci-agent run 42793810724, headSha 947c06f, failed with `CreateGraph with a view parent: err = <nil>`, `CreateGraph with view children: err = fakedriver: Scan got 2 destinations...` (the view children were inserted) and `FindOrCreate: err = momo: not found` (the view was inserted into and re-selected). The Copy case did not fail there: at 4a9d64f a query built through `Insert` carried the error set by the constructor, so `Copy` already refused it; the case guards the directly built query and passes before and after.
- Not run: the compile-fail design of the plan (dropped by the decision).
