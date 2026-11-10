# momo-02 Task 30: Field permissions at query build time (S-71, untyped paths)

Status: open
Created: 2026-11-04
Epic: momo-02-generator
Sprint: -
Carried: 007-sprint-7-finish-every-open-backlog-story-auditing-each — User decision 2026-11-10 (decisions.md): the untyped AssignAny/Permission design is not built; typed paths enforce permissions. TestFieldPermissionsRuntime exists but is not the plan's test; criterion stays unticked, superseded.

## Description

Implements plan momo-02-generator Task 30: Field permissions at query build time (S-71, untyped paths) (`.procoder/plans/momo-02-generator.md:3512`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-71. Files: `momo/permissions_runtime_test.go`, `momo/permissions.go`, `gen/testdata/permissions/model/momo_gen.go`. Contributes to S-71; those criteria are verified in the stories that define their named tests. Scheduled after momo-05: runtime permission checks use the Update/Insert builders.

## Acceptance criteria

- [ ] `TestFieldPermissionsRuntime` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

Carry reason: user decision 2026-11-10 (decisions.md): typed paths enforce permissions; ColumnRef interface not extended.

Audit: the whole-model half of the task is already enforced by the builders: `columnAssignable` (db_result.go:24) makes `Update.Model` skip read-only, create-only and immutable columns and `Insert.Model` skip read-only ones; `Update.Set` of a restricted column is refused with `ErrValidation` (update.go:175). Neither `AssignAny` nor `Permission()` exists, and no code in the product has an untyped assign path that would call them (typed `Set` is compile-checked). Adding them changes the exported `ColumnRef` interface, and "converts v to the column's Go type" needs a conversion rule (the plan says: the same as `ParseEq`, i.e. from strings).

Options: (1) add both as in the plan (API addition, docs and reference change, conversion rule decided); (2) keep the shipped design and close this story with the runtime test below as the evidence.

Built meanwhile: `TestFieldPermissionsRuntime` in col_permissions_runtime_test.go (a hand-written `Meta` with read-only, create-only and immutable columns; asserts the SQL of `Update.Model` and `Insert.Model` and the `ErrValidation` of `Update.Set` on each restricted column). It passes in ci-agent run 42773371002, headSha 5acef91. It is not the plan's test (no `AssignAny`, no generated gen/testdata/permissions/model/momo_gen.go), so the criterion stays unticked.

