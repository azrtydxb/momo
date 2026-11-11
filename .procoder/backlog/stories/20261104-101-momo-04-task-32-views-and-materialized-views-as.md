# momo-04 Task 32: Views and materialized views as read-only models

Status: done 2026-11-11
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: 009-fix-every-open-github-issue-86-to-108-the-kuvryn-orbit-port
Carried: 007-sprint-7-finish-every-open-backlog-story-auditing-each — TestViewInsertDoesNotCompile and the mutation-builders-do-not-compile clause of S-37 belong to the compile-time design dropped by the 2026-11-10 runtime-guard decision; the user must retire or reword them. RefreshMaterializedView, the view-model-has-no-table fix and TestViewsAsModels (adapted) are built; green ci-agent run 42838538032.

## Description

Implements plan momo-04-migrations Task 32: Views and materialized views as read-only models (`.procoder/plans/momo-04-migrations.md:3279`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-37. Files: `internal/compat/viewmodels/viewmodels.go`, `views.go`.

## Acceptance criteria

- [x] [S-37] `TestViewModelsNotWritable` (meta_view_test.go:44) replaces `TestViewInsertDoesNotCompile` for the runtime guard: `Insert.Model`, `Insert.Models`, `Insert.Values`, `Upsert`, `Update.Set`, `Update.Model` and `SoftDelete` on a view model fail with `ErrValidation` naming the model, and reads are unaffected.
- [x] [S-37] `TestViewsAsModels` as shipped (migrate/postgres/views_test.go:105, PostgreSQL): hand-written view models, a view, a view over a view and a materialized view created by migration statements, queried as read-only models, the materialized view stale until `RefreshMaterializedView` (also `CONCURRENTLY` and schema-qualified), `Insert` on a view model refused with `ErrValidation`, replacement of the base view rebuilding its dependent, and everything dropped in dependency order.
- [x] [S-37] A view and a materialized view are created by migrations, queried as read-only models and refreshed by `momo.RefreshMaterializedView` (`TestViewsAsModels`, `TestRefreshMaterializedView` in meta_views_test.go:18); mutation builders on a view model are refused at build time by the runtime guard with `ErrValidation` instead of failing to compile (`TestViewModelsNotWritable`); `TestViewsAsModels` drops the views in dependency order and asserts none remain, so it passes only when that order is valid.
- [x] `procoder check` gate is clean and the task's commit is made (the task's code is on main at 4781864; the lint job 130341061844 of ci.yml run 42965315635 is success; the commit gate ran on the sprint 9 bookkeeping commit)

## Evidence

### Sprint 7 (2026-11-10)

Carry reason: blocked on story 20261104-050 (momo-02 task 26), which defines `momo.View`, `WritableModeler` and the generator output this task consumes. `git grep -n "WritableModeler\|RefreshMaterializedView"` finds nothing in the code, so `internal/compat/viewmodels`, the compile-fail fixture and `views.go` cannot be written yet. The migration half already works: views and materialized views are created, replaced and dropped in dependency order (migrate/order_test.go, migrate/postgres/managed_objects_test.go) and drift now compares their queries.

### Sprint 7 second pass (2026-11-10)

Built, test-first. Red: ci-agent run 42828122935, headSha 10b7955 (stub helper, tests only): `TestRefreshMaterializedView`, `TestRefreshMaterializedViewRefusals`, `TestViewModelDeclaresNoTable` and `TestViewsAsModels` (migrate/postgres/views_test.go, integration) FAIL. Green: ci-agent run 42838538032, headSha 5e36833, all 9 jobs.

- `momo.RefreshMaterializedView(ctx, q, name, concurrently)` (meta_views.go): `REFRESH MATERIALIZED VIEW [CONCURRENTLY]`, quoted, `schema.name` split, PostgreSQL only (ErrUnsupported elsewhere), ErrValidation on a bad name.
- Gap found and fixed: the schema builder emitted a table for every `momo.View` model, so a migration would have issued `CREATE TABLE` for the view next to its `CREATE VIEW`. A view model now adds no table (gen/schema.go); the emitter keeps its table separately. `TestViewModelDeclaresNoTable`.
- `TestViewsAsModels`: a view, a view over a view and a materialized view are created by migration statements, read as models, the materialized view is stale until `RefreshMaterializedView`, `Insert` on a view model returns ErrValidation, replacing the base view rebuilds the dependent, and dropping everything works in dependency order (PostgreSQL would refuse a wrong order).

Still unmet, so the story is carried: `TestViewInsertDoesNotCompile` and the "mutation builders do not compile" clause of S-37 belong to the compile-time design that the 2026-11-10 decision dropped in favour of the runtime guard. The criteria are not rewritten here; they need the user to retire or reword them. `TestViewsAsModels` exists in an adapted form (hand-written view models in the test, runtime guard instead of compile failure), not as the plan wrote it. `internal/compat/viewmodels` was not built: the hand-written models in the test cover it.

### Sprint 7 review fixes (2026-11-10)

- View models in the ecosystem plugins: `Graph.TableOf` (gen/emit_meta.go) reads the view tables first; the grpc, gosrc, graphql, viz and openapi lookups use it. Red: ci-agent run 42843095672 (throwaway branch, implementation reverted, headSha 5dad868): `TestViewModelKeepsItsKey` (grpc), `TestSinglePKOfViewModel`, `TestSingleIntPKOfViewModel` and `TestKeyOfViewModel` FAIL. The openapi test cannot fail with the old lookup (the pk tag marks the field too), so it only guards.
- Name parts over 63 bytes are refused (`TestRefreshMaterializedViewRefusals`) and a view model sharing a table name with a table or view model is diagnosed (`TestViewModelSharingATableNameIsDiagnosed`): both FAIL in the same red run.
- Added `CONCURRENTLY` and schema-qualified refreshes to `TestViewsAsModels`, and `TestBelongsToViewModelAddsNoForeignKey`; these pass on the old code too (coverage, not a fix).
- Green: ci-agent run 42843104817, headSha f13269b.

## Reworded 2026-11-11 (user decision)

User decision of 2026-11-11, recorded in .procoder/ask/decisions.md, section "Carried plan stories": reword the criteria to the design that shipped, cite CI evidence, and close the story. The `Carried:` lines above stay as the history.

- Original: `TestViewInsertDoesNotCompile` exists as written in the plan, failed before the implementation, and passes now
  - Reason: the compile-time design (a `WritableModeler` split and a compile-fail fixture) was dropped by the 2026-11-10 decision in favour of a runtime guard, so there is no `TestViewInsertDoesNotCompile`; `TestViewModelsNotWritable` pins the runtime guard.
- Original: `TestViewsAsModels` exists as written in the plan, failed before the implementation, and passes now
  - Reason: the shipped test uses hand-written view models and the runtime guard instead of the plan's `internal/compat/viewmodels` package and compile failure.
- Original: [S-37] ... mutation builders on them do not compile; ...
  - Reason: replaced by the runtime guard, as above.

### Sprint 9 CI evidence (2026-11-11)

Jobs of ci.yml run 42965315635 on main (headSha 47818647165f0231b1117bb6ebe2bce67dbed444, conclusion success). The job logs carry package-level `ok` lines, not `-v` output: unit (1.26) 130341061373 (`make test`), race (1.26) 130341061707 (`make test-race`), integration (1.27, 0.8.7-pg17, postgres) 130341061675 (`make test-integration-postgres`: MOMO_REQUIRE_INTEGRATION=1 and `-tags integration -race`, so a skipped database test fails), windows 130341061640, bench-module 130341061885 (`make test-bench-module`), lint 130341061844.

- `TestViewModelsNotWritable` and `TestRefreshMaterializedView` are in package momo_test (no build tag): the root package is `ok` in the unit job (39.157s), the race job, the windows job and the PostgreSQL integration job.
- `TestViewsAsModels` is `//go:build integration`: momo/migrate/postgres is `ok` (127.624s) in the PostgreSQL integration job 130341061675 (MOMO_REQUIRE_INTEGRATION=1, so no skip).
- Earlier green runs for the same tests: ci-agent run 42838538032 (headSha 5e36833) and 42843104817 (headSha f13269b).
