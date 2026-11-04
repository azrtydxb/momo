# momo-02 Task 27: Companion schema declarations (structured, read statically)

Status: open
Created: 2026-11-04
Epic: momo-02-generator
Sprint: -

## Description

Implements plan momo-02-generator Task 27: Companion schema declarations (structured, read statically) (`.procoder/plans/momo-02-generator.md:2876`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: supporting work for the plan. Files: `momo/companion.go`, `momo/companion_test.go`, `gen/companion.go`, `gen/companion_schema.go`, `gen/companion_schema_test.go`, `gen/testdata/fixtures/companionschema/s.go` ….

## Acceptance criteria

- [ ] `TestCompanionSchemaMethods` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestCompanionSchemaDiagnostics` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestCompanionSchemaDSLCompiles` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

