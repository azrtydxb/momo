# momo-02 Task 30: Field permissions at query build time (S-71, untyped paths)

Status: done 2026-11-11
Created: 2026-11-04
Epic: momo-02-generator
Sprint: 009-fix-every-open-github-issue-86-to-108-the-kuvryn-orbit-port
Carried: 007-sprint-7-finish-every-open-backlog-story-auditing-each — User decision 2026-11-10 (decisions.md): the untyped AssignAny/Permission design is not built; typed paths enforce permissions. TestFieldPermissionsRuntime exists but is not the plan's test; criterion stays unticked, superseded.

## Description

Implements plan momo-02-generator Task 30: Field permissions at query build time (S-71, untyped paths) (`.procoder/plans/momo-02-generator.md:3512`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-71. Files: `momo/permissions_runtime_test.go`, `momo/permissions.go`, `gen/testdata/permissions/model/momo_gen.go`. Contributes to S-71; those criteria are verified in the stories that define their named tests. Scheduled after momo-05: runtime permission checks use the Update/Insert builders.

## Acceptance criteria

- [x] [S-71] `TestFieldPermissionsRuntime` (col_permissions_runtime_test.go:50) as shipped: on a hand-written `Meta` with read-only, create-only and immutable columns, `Update.Model` and `Insert.Model` leave the restricted columns out of their SQL and `Update.Set` of each restricted column returns `ErrValidation`. Typed paths enforce field permissions; the untyped `AssignAny` and `Permission()` are not built (user decision 2026-11-10).
- [x] `procoder check` gate is clean and the task's commit is made (the task's code is on main at 4781864; the lint job 130341061844 of ci.yml run 42965315635 is success; the commit gate ran on the sprint 9 bookkeeping commit)

## Evidence

Carry reason: user decision 2026-11-10 (decisions.md): typed paths enforce permissions; ColumnRef interface not extended.

Audit: the whole-model half of the task is already enforced by the builders: `columnAssignable` (db_result.go:24) makes `Update.Model` skip read-only, create-only and immutable columns and `Insert.Model` skip read-only ones; `Update.Set` of a restricted column is refused with `ErrValidation` (update.go:175). Neither `AssignAny` nor `Permission()` exists, and no code in the product has an untyped assign path that would call them (typed `Set` is compile-checked). Adding them changes the exported `ColumnRef` interface, and "converts v to the column's Go type" needs a conversion rule (the plan says: the same as `ParseEq`, i.e. from strings).

Options: (1) add both as in the plan (API addition, docs and reference change, conversion rule decided); (2) keep the shipped design and close this story with the runtime test below as the evidence.

Built meanwhile: `TestFieldPermissionsRuntime` in col_permissions_runtime_test.go (a hand-written `Meta` with read-only, create-only and immutable columns; asserts the SQL of `Update.Model` and `Insert.Model` and the `ErrValidation` of `Update.Set` on each restricted column). It passes in ci-agent run 42773371002, headSha 5acef91. It is not the plan's test (no `AssignAny`, no generated gen/testdata/permissions/model/momo_gen.go), so the criterion stays unticked.

## Reworded 2026-11-11 (user decision)

User decision of 2026-11-11, recorded in .procoder/ask/decisions.md, section "Carried plan stories": reword the criteria to the design that shipped, cite CI evidence, and close the story. The `Carried:` lines above stay as the history.

- Original: `TestFieldPermissionsRuntime` exists as written in the plan, failed before the implementation, and passes now
  - Reason: the plan's test drives an untyped `AssignAny`, a `Permission()` method on `ColumnRef` and a generated gen/testdata/permissions/model/momo_gen.go. The 2026-11-10 decision (decisions.md) keeps the typed design: `columnAssignable` (db_result.go) skips restricted columns in `Update.Model` and `Insert.Model`, `Update.Set` refuses a restricted column (update.go), and typed `Set` is compile-checked, so no untyped assign path exists to guard. The test of that name that shipped covers the behaviour that exists.

### Sprint 9 CI evidence (2026-11-11)

Jobs of ci.yml run 42965315635 on main (headSha 47818647165f0231b1117bb6ebe2bce67dbed444, conclusion success). The job logs carry package-level `ok` lines, not `-v` output: unit (1.26) 130341061373 (`make test`), race (1.26) 130341061707 (`make test-race`), integration (1.27, 0.8.7-pg17, postgres) 130341061675 (`make test-integration-postgres`: MOMO_REQUIRE_INTEGRATION=1 and `-tags integration -race`, so a skipped database test fails), windows 130341061640, bench-module 130341061885 (`make test-bench-module`), lint 130341061844.

- `TestFieldPermissionsRuntime` is in col_permissions_runtime_test.go:50 (package momo_test, no build tag). The root package `github.com/azrtydxb/momo` is `ok` in the unit job (39.157s), the race job (135.480s), the windows job (68.391s) and the PostgreSQL integration job (937.158s).
- Earlier run for the same test: ci-agent run 42773371002, headSha 5acef91.
