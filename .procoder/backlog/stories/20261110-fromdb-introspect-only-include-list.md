# --from-db: an include list (introspect.only)

Status: done 2026-11-10
Created: 2026-11-10
Epic: -
Sprint: 008-sprint-8-fix-the-kuvryn-orbit-port-issues-73-to-79-and-the
Type: story

## Description

`momo generate --from-db` models every table except those matched by `introspect.skip`. A project that moves onto momo one module at a time needs models only for the tables it has moved. Add `introspect.only: [glob, ...]`, the inverse of `skip`, with the same effect on the init snapshot and `migrate.exclude`. GitHub issue #73.

## Acceptance criteria

- [x] the tests are shown failing on CI before the change
- [x] `introspect.only` limits `--from-db` models, the init snapshot and `migrate.exclude` to the matching tables; a test shows a table outside `only` gets no model and is excluded from migrate
- [x] `only` and `skip` together: `skip` applies within `only`; documented in docs/importers.md or the --from-db docs and in docs/upgrading.md under 0.7.0

## Evidence

Red, run 42904286499 on the throwaway branch worktree-agent-a74b43ddb346ed43c-red (commit d1d4052: config fields and a no-op migrate.applyOnly only; the branch is deleted): unit failed TestOnlyLimitsModelsAndSnapshot ("files = 2, want only the users model"), TestOnlyDropsObjectsOfOtherTables, TestIntrospectScopeValidation (only glob "[" accepted), migrate TestApplyOnlyHidesTablesOutsideTheList; integration postgres failed TestGenerateFromDBOnly ("tickets_model.go: a table outside only ... got a model"). TestOnlyAndSkipTogether passed in the red run, because skip alone already removes orders; it pins that skip still applies inside only.

Green, run 42913355678 (commit 141e504, ci-agent: unit, integration postgres, mysql, apidiff-docs, lint all success). Tests: gen/fromdb TestOnlyLimitsModelsAndSnapshot (no model, no snapshot table), TestOnlyAndSkipTogether (skip applies within only), TestOnlyDropsObjectsOfOtherTables, TestIntrospectScopeValidation; migrate TestApplyOnlyHidesTablesOutsideTheList (a table outside only is hidden from drift, baseline and diff, so it is excluded from migrate); cli TestGenerateFromDBOnly against PostgreSQL (tickets and a skipped table get no model, the init snapshot has neither, baseline and drift exit 0 with the other tables unmanaged).

Docs: docs/upgrading.md "Upgrading to 0.7.0" (only, skip within only), docs/migrations.md "The introspect section". Code: gen/fromdb/config.go (Only, outsideOnly), gen/fromdb/emit.go collectTables, migrate/drift.go applyOnly via Engine.hide, migrate.Config.OnlyTables set from introspect.only in cli/migrate.go engineOptions.

Follow-up review, both rounds. Review round 1: red run 42920681315 (commit 539df4d) and, for the function-dependency tests, run 42922252254 (commit 11c1828), both on throwaway branches since deleted; green ci-agent run 42932039383 on the merged commit 3c09de5 (all 9 jobs success). Pinned and shown red: TestDiffOnlyTablesNeverDropsAnUnmanagedTable (migrate diff planned drop_table c), TestOnlyAndSkipMatchSchemaQualifiedNames and TestGenerateFromDBOnlySchemaQualified (only: ["app.*"] wrote no model), TestOnlyRejectsAnEmptyGlob, TestPostgresInspectOnlySkipsOtherTables (inspection read every table, and the triggers and policies of tables outside only).

Review round 2: red run 42935148807 (commit f76275a, the new tests plus a revert of the three code lines they pin, throwaway branch since deleted); green ci-agent run 42938784308 on the merged commit 35ce819 (sprint-8 906fcb0 merged in; all 9 jobs success). Pinned and shown red: TestDiffOnlyTablesNeverCreatesAnUnmanagedTable (the desired-side filter: planned CREATE TABLE c), TestDiffRejectsBadOnlyTables (the error named Config.Exclude, an empty entry hid every table), TestIntrospectOnlyReachesInspection (introspect.only to InspectOptions.Only wiring), TestPostgresInspectOnlyMatchesSchemaQualifiedNames (a table in a schema other than the current one). TestPostgresInspectOnlyAppliesToPartitionChildrenByName pins what really happens (a partition child is a table of its own, so only: [orders] leaves out orders_2024) and passed in the red run by design; TestIntrospectOnlyEmptyEntryIsRejected passes because config loading already validates (config.Load calls Introspect.Validate), the migrate side now also rejects an empty entry. Docs: docs/upgrading.md 0.7.0 "Library users" note (migrate.Config.OnlyTables, migrate.InspectOptions.Only, MySQL and SQLite inspectors read every table), partition children, desired-side filter, skip matching schema.name; docs/migrations.md; docs/reference.md (migrate.MatchObject) from the regenerated-docs artifact of run 42935300015 (headSha dfe3923, whose code is unchanged in the final tree).
