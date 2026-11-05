# migrate: phantom check-expression drift after applying a generated migration

Status: done
Created: 2026-11-05
Epic: momo-04-migrations-core-postgresql
Sprint: -
Type: bug
Severity: s2

## Description

Reproduction: build the schema of internal/testmodels/example with gen.Load and gen.BuildSchema, run Engine.Diff from an empty directory, apply the generated files with Engine.Apply, then call Engine.Drift. Drift reports the check documents.documents_title_nonempty as changed ("expr differ") although nothing was touched. A second Engine.Diff correctly finds nothing, because it compares the snapshot with the same declared text.

Observed: declared expression `length(title) > 0`; the inspected expression is `(length((title)::text) > 0)`. Drift compares the snapshot, which holds the text as the model declared it, with the live database, which holds what PostgreSQL stored.

Expected: a freshly applied migration is clean under Drift.

Root cause: pg_get_constraintdef, pg_get_expr and pg_get_indexdef respell stored text: extra parentheses, `(col)::text` casts on varchar columns, `IN (...)` as `= ANY (ARRAY[...])`, `BETWEEN` as two comparisons, `'x'::text` literals. schema.NormalizeExpr only strips outer parentheses and literal casts, so it cannot make these equal. The Canonicalizer hook (Baseline) covered views, functions, triggers and policies but not table text, and Drift did not canonicalize at all. The same phantom applies to column defaults, generated expressions and index predicates and expressions, and Baseline would have refused such a database.

The vector extension report in the same run is not a bug here: gen.BuildSchema adds `CREATE EXTENSION vector` to the schema when a model has a vector column (the example models do), and Diff emits it. Inspection reports every installed extension, so a schema that declares none (hand-written test schemas on a database where pgvector is installed) lists vector as unmanaged; that is expected and handled with a declared extension or Config.Exclude.

Fix: postgres Canonicalize now also round-trips table check expressions, column defaults and generated expressions, and index predicates and expression parts through the scratch schema. Engine.Drift canonicalizes a copy of the snapshot's table text before comparing (views, functions, triggers and policies keep their existing rules); a role without CREATE on the database falls back to the declared text.

## Acceptance criteria

- [x] a regression test pins the fix: red before the change, green after
- [x] Engine.Drift after applying a migration generated from the example models is clean and a second Diff is empty
- [x] IN and BETWEEN checks, defaults, generated columns and index predicates and expressions do not drift either, and a real check change is still reported
- [x] Drift still works for a role without the canonicalize privilege
- [x] unit tests cover the table-text rewrite

## Evidence

- Red before the change: TestLifecycleExampleModelsNoPhantomDrift failed with `Changed:[{Object:{Kind:check Name:documents.documents_title_nonempty} Detail:expr differ}]`, inspected text `(length((title)::text) > 0)`.
- Green after: go test -tags integration -run 'TestLifecycle|TestDriftWithout' ./migrate/postgres/ passes (migrate/postgres/lifecycle_example_test.go).
- Unit: TestHasTableText and TestRewriteTables in migrate/postgres/canonicalize_tables_internal_test.go.
- Full runs recorded in the commit report.
