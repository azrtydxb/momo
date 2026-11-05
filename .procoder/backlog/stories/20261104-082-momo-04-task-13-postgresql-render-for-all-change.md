# momo-04 Task 13: PostgreSQL render for all change kinds

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Note (Task 9 review): use `migrate.OrderErr` (or `OrderWithViews`), not `Order`, and surface the cycle error it returns.

Implements plan momo-04-migrations Task 13: PostgreSQL render for all change kinds (`.procoder/plans/momo-04-migrations.md:1401`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-12, S-37, S-38, S-39. Files: `migrate/postgres/render.go`. Contributes to S-37, S-38; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] Function render emits `STRICT` when `Function.Strict` and `SECURITY DEFINER` when `Function.SecurityDefiner` (both are inspected since Task 12) and the round trip has no diff (evidence: `TestRenderFunctionStrictSecurityDefiner` and integration `TestFunctionStrictSecurityDefinerRoundTrip` in `migrate/postgres/`; failed before the render change, passed after; mutation of the SECURITY DEFINER branch fails both)

- [ ] `TestPostgresObjects` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestPostgresConcurrentIndexNoTx` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestManagedFunctionsTriggersPartitions` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-12] `TestPostgresObjects` on real PostgreSQL+pgvector: vector extension, enum, partial index, expression index, HNSW index with opclass and a CONCURRENTLY index apply and round-trip with no diff; fails if CONCURRENTLY runs inside a transaction.
- [ ] [S-39] `TestManagedFunctionsTriggersPartitions`: a function, trigger and partitioned table are created by migrations, round-trip with no diff, and a changed function body produces CREATE OR REPLACE; fails if they are reported as unmanaged.
- [x] `procoder check` gate is clean and the task's commit is made
- [x] PostgreSQL render emits `Column.Collate` (`COLLATE`) and `Column.Comment` (`COMMENT ON COLUMN`) for create and alter, with a round-trip test against inspection

## Evidence

- Note: this task carries the inspection and render part of S-74 (FK ON DELETE/ON UPDATE on every dialect, and no FK in inspection for the no-constraint option); the SQLite render story carries the SQLite half. Task 8 (story 077) covers only the diff part.

- Done (commit 7d1a725): `migrate/postgres/render.go` renders all 32 change kinds; tests in `render_test.go`, `roundtrip_test.go`, `managed_objects_test.go`. Before the implementation the package failed to build (`mpg.Dialect{}.Render undefined`); after it, `go test -tags integration -race ./migrate/... ./schema/...` passes. Mutation check: dropping `NoTx` from CONCURRENTLY indexes failed `TestPostgresObjects` and `TestPostgresConcurrentIndexNoTx`.
- Round trip on live PostgreSQL+pgvector (model, Diff from empty, OrderWithViews, Render, apply, Inspect, schema.Equal, second Diff is empty): vector extension, enum, partial and expression index, HNSW with opclass and `m`, CONCURRENTLY index, FKs with ON DELETE/ON UPDATE (including a self-reference), collate "C", escaped comment, check, unique, and alter_column with type/collate/comment/default changes.
- Still open: `TestPostgresObjects` and `TestManagedFunctionsTriggersPartitions` are not the plan's literal text because the Engine and `migratetest` (Tasks 15+) do not exist; they drive Diff, OrderWithViews, Render and the driver directly. The S-39 criterion stays unchecked: it needs story 081 (function, trigger, policy and view inspection) for the read-back, no-diff and not-unmanaged assertions; here the function, trigger and partitions are applied, the trigger is shown firing, and a body change is shown to render CREATE OR REPLACE FUNCTION.
- S-74 render part: FK ON DELETE/ON UPDATE render and round trip. DEFERRABLE is not rendered because `schema.ForeignKey` has no such field; the no-constraint FK option is not covered here.
- Deviations: alter_enum_add_value appends (no BEFORE/AFTER) because Change carries only the new value; plan fixture `lower(state::text)` is not IMMUTABLE on an enum, so the test uses `lower(title)`; round-trip tests pin extension versions and FormatVersion because `schema.Equal` compares both literally.
- Deferred checks now live in story 085 (Engine): `TestPostgresObjects` through the Engine, and the S-39 no-diff and not-unmanaged assertions of `TestManagedFunctionsTriggersPartitions`. The function and trigger inspection they depend on is story 081; story 081's file was not edited from this branch.
- Review fixes: `schema.Equal` now ignores FormatVersion and compares an extension version only when both sides set it (`TestEqualExtensionVersionAndFormatVersion`); the round-trip tests no longer pin versions. The dollar-tag check also refuses a body ending in `$momo`; policy role keywords are case-insensitive; `Trigger.Function` is validated; an unknown change kind errors; plan Constraints line added.
