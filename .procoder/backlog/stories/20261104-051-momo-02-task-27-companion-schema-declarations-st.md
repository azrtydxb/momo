# momo-02 Task 27: Companion schema declarations (structured, read statically)

Status: open
Created: 2026-11-04
Epic: momo-02-generator
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

Implements plan momo-02-generator Task 27: Companion schema declarations (structured, read statically) (`.procoder/plans/momo-02-generator.md:2876`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: supporting work for the plan. Files: `momo/companion.go`, `momo/companion_test.go`, `gen/companion.go`, `gen/companion_schema.go`, `gen/companion_schema_test.go`, `gen/testdata/fixtures/companionschema/s.go` …. Scheduled after momo-05 and momo-02 Task 26; also needs momo-04 Task 1 (story 070).

## Acceptance criteria

- [x] `TestCompanionSchemaMethods` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestCompanionSchemaDiagnostics` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestCompanionSchemaDSLCompiles` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

Carry reason: the feature shipped under other names; only the `momo.View` marker rule is missing, and that belongs to story 050 (Needs decision).

- The seven structured companion methods exist as no-ops on `*momo.Config`: `View`, `Function`, `Trigger`, `Policy`, `RLS`, `Partition`, `PartitionOf` (meta_companion.go:59-85, and `View` just above). Their compile check is `TestCompanionManagedObjectsCompile` (meta_companion_test.go:47), the shipped counterpart of `TestCompanionSchemaDSLCompiles`.
- The static reader accepts the structured literals: `schema` type table at gen/companion.go:48-54, `CompanionResult` carries the objects, and `TestCompanionStructuredArguments` (gen/companion_test.go:57) with the fixture gen/testdata/fixtures/companionstruct/ok.go asserts functions, triggers, policies, RLS and partition are read. Hostile shapes (identifiers, non-literals) are diagnosed by `TestCompanionHostileShapes` (gen/companion_test.go:89), the counterpart of `TestCompanionSchemaDiagnostics`.
- `BuildSchema` merges them into the schema (`mergeObjects`, gen/schema.go:561; asserted for views and policies at gen/schema_test.go:399-427).
- The `momo.View` marker rule: the marker now exists (story 050, user decision 2026-11-10 in decisions.md) and is read by the generator (gen/model.go, `TestViewMarkerEmission`, ci-agent run 42784895483, headSha d5d4696). The plan's extra diagnostic `view model <GoName> has no c.View for <name>` is not built: the runtime guard does not need a companion `c.View`, and requiring one would reject views declared outside the Go models.

The plan's three test names (`TestCompanionSchemaMethods`, `TestCompanionSchemaDiagnostics`, `TestCompanionSchemaDSLCompiles`) do not exist; their intent is covered by the tests above. Criteria left unticked for the integrator to carry or retire.

### Sprint 7 close-out (2026-11-10)

The three plan tests now exist: `TestCompanionSchemaMethods` and `TestCompanionSchemaDiagnostics` (gen/companion_schema_test.go, fixtures gen/testdata/fixtures/companionschema and companionschemabad) and `TestCompanionSchemaDSLCompiles` (meta_companion_test.go). Deviations from the plan text: the fixtures carry the licence header, the view model uses the shipped shape (`momo.Model` plus `momo.View`), and the plan's `view model X has no c.View` diagnostic is not built (see above: a view can be declared outside the Go models).

The code existed before the tests, so red was shown by breaking it on throwaway branches (tests identical):

- ci-agent run 42828247513, headSha afb8fa2 (RLS merge into the table disabled, structured-argument diagnostic silenced): `TestCompanionSchemaMethods` and `TestCompanionSchemaDiagnostics` FAIL.
- ci-agent run 42828291415, headSha 5d49263 (`Config.RLS` removed): the root package fails to build (`c.RLS undefined`), so `TestCompanionSchemaDSLCompiles` cannot pass.

Green: ci-agent run 42838538032, headSha 5e36833, all 9 jobs. `procoder check` over the files changed against origin/sprint-7: 27 clean, 0 blocking.
