# --from-db: write only the functions, policies and triggers the modeled tables use

Status: done 2026-11-10
Created: 2026-11-10
Epic: -
Sprint: 008-sprint-8-fix-the-kuvryn-orbit-port-issues-73-to-79-and-the
Type: story

## Description

`--from-db` writes every database function into the `MomoConfig` of the alphabetically first model, so a three-table port carries about 450 lines of unrelated plpgsql, and its header hash changes on every migration. Default (integrator decision 2026-11-10): emit only the functions that a modeled table's triggers, defaults, checks or policies reference, each on the model of the table that uses it. Add `introspect.functions: all` for the old behaviour. Database-wide objects that no modeled table references are not written. GitHub issue #74.

## Acceptance criteria

- [x] the tests are shown failing on CI before the change
- [x] with three modeled tables, a function no modeled table references is not written; a function a modeled table's trigger calls is written on that table's model; a test pins both
- [x] `introspect.functions: all` restores the old behaviour
- [x] the generated file of a model does not change when an unrelated function or table changes in the database (a test regenerates after adding an unrelated function and compares)

## Evidence

Red, run 42904286499 on the throwaway branch worktree-agent-a74b43ddb346ed43c-red (commit d1d4052: config fields and a no-op migrate.applyOnly only; the branch is deleted): unit failed TestFunctionsOnlyThoseModeledTablesUse ("a function no modeled table uses was written", "beta lacks Function touch", "alpha, the first model, carries touch"), TestModelFilesStableAgainstUnrelatedChanges (alpha changed), TestIntrospectScopeValidation (functions: "some" accepted). TestFunctionsAllRestoresOldBehaviour passed in the red run by design: functions: all is the unchanged old behaviour and the test pins it.

Green, run 42913355678 (commit 141e504, ci-agent: unit, integration postgres, mysql, apidiff-docs, lint all success). Tests (gen/fromdb/scope_test.go): TestFunctionsOnlyThoseModeledTablesUse pins that unrelated_fn is written nowhere (and is not in the snapshot), touch (trigger) and its callee touch_helper sit on beta, next_code (column default) on gamma, can_read (policy) on alpha, and the first model carries none of the others; TestFunctionsAllRestoresOldBehaviour; TestModelFilesStableAgainstUnrelatedChanges regenerates after adding an unrelated function, a new table and its trigger and compares the three model files byte for byte.

Triggers and policies are now also written on their own table model (same file stability reason); the functions of views and of triggers of unmodeled tables stay on the first model. Docs: docs/upgrading.md "Upgrading to 0.7.0", docs/migrations.md "The introspect section". Code: gen/fromdb/functions.go, gen/fromdb/emit.go globals.

Follow-up review, both rounds. Review round 1: red run 42920681315 (commit 539df4d) and, for the function-dependency tests, run 42922252254 (commit 11c1828), both on throwaway branches since deleted; green ci-agent run 42932039383 on the merged commit 3c09de5 (all 9 jobs success). Pinned and shown red: TestFunctionsFollowArgumentDefaultsAndRegprocLiterals (a function reached through another function's argument default, or a regproc or regprocedure literal, was left out); TestFunctionsCannotFollowDynamicSQL documents the EXECUTE limit and passes by design.

Review round 2: red run 42935148807 (commit f76275a, the new tests plus a revert of the three code lines they pin, throwaway branch since deleted); green ci-agent run 42938784308 on the merged commit 35ce819 (sprint-8 906fcb0 merged in; all 9 jobs success). Pinned and shown red: TestFunctionsFollowQuotedAndQualifiedRegprocReferences (a quoted name holding a dot, a pg_catalog-qualified cast and to_regproc('name') were missed; the scanner is now two regexps plus a small identifier parser). Documented in docs/upgrading.md 0.7.0 and docs/migrations.md: what is followed, and that dynamic SQL cannot be, with introspect.functions: all as the workaround.
