# momo-06 Task 3: Relation fixture models, schema and seed data

Status: open
Created: 2026-11-04
Epic: momo-06-relations
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-06-relations Task 3: Relation fixture models, schema and seed data (`.procoder/plans/momo-06-relations.md:477`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-6, S-76, S-82. Files: `internal/relfixture/models/model.go`, `internal/relfixture/models/momo_gen.go`, `internal/relfixture/author/momo_gen.go`, `internal/relfixture/document/momo_gen.go`, `internal/relfixture/comment/momo_gen.go`, `internal/relfixture/tagging/momo_gen.go` …. Contributes to S-76, S-82; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestRelfixtureSchemaApplies` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-6, part] this story's tests pass for the part of S-6 this task delivers; the full criterion is verified in story 146.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

