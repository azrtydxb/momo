# momo-04 Task 32: Views and materialized views as read-only models

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

Implements plan momo-04-migrations Task 32: Views and materialized views as read-only models (`.procoder/plans/momo-04-migrations.md:3279`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-37. Files: `internal/compat/viewmodels/viewmodels.go`, `views.go`.

## Acceptance criteria

- [ ] `TestViewInsertDoesNotCompile` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestViewsAsModels` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-37] `TestViewsAsModels`: a view and a materialized view are created by migrations, queried as read-only models, refreshed by the helper; mutation builders on them do not compile; fails if a view is dropped before its dependents.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

### Sprint 7 (2026-11-10)

Carry reason: blocked on story 20261104-050 (momo-02 task 26), which defines `momo.View`, `WritableModeler` and the generator output this task consumes. `git grep -n "WritableModeler\|RefreshMaterializedView"` finds nothing in the code, so `internal/compat/viewmodels`, the compile-fail fixture and `views.go` cannot be written yet. The migration half already works: views and materialized views are created, replaced and dropped in dependency order (migrate/order_test.go, migrate/postgres/managed_objects_test.go) and drift now compares their queries.

### Sprint 7 second pass (2026-11-10)

Built, test-first. Red: ci-agent run 42828122935, headSha 10b7955 (stub helper, tests only): `TestRefreshMaterializedView`, `TestRefreshMaterializedViewRefusals`, `TestViewModelDeclaresNoTable` and `TestViewsAsModels` (migrate/postgres/views_test.go, integration) FAIL. Green: ci-agent run 42838538032, headSha 5e36833, all 9 jobs.

- `momo.RefreshMaterializedView(ctx, q, name, concurrently)` (meta_views.go): `REFRESH MATERIALIZED VIEW [CONCURRENTLY]`, quoted, `schema.name` split, PostgreSQL only (ErrUnsupported elsewhere), ErrValidation on a bad name.
- Gap found and fixed: the schema builder emitted a table for every `momo.View` model, so a migration would have issued `CREATE TABLE` for the view next to its `CREATE VIEW`. A view model now adds no table (gen/schema.go); the emitter keeps its table separately. `TestViewModelDeclaresNoTable`.
- `TestViewsAsModels`: a view, a view over a view and a materialized view are created by migration statements, read as models, the materialized view is stale until `RefreshMaterializedView`, `Insert` on a view model returns ErrValidation, replacing the base view rebuilds the dependent, and dropping everything works in dependency order (PostgreSQL would refuse a wrong order).

Still unmet, so the story is carried: `TestViewInsertDoesNotCompile` and the "mutation builders do not compile" clause of S-37 belong to the compile-time design that the 2026-11-10 decision dropped in favour of the runtime guard. The criteria are not rewritten here; they need the user to retire or reword them. `TestViewsAsModels` exists in an adapted form (hand-written view models in the test, runtime guard instead of compile failure), not as the plan wrote it. `internal/compat/viewmodels` was not built: the hand-written models in the test cover it.
