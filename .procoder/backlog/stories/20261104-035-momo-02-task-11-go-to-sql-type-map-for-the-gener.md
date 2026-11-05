# momo-02 Task 11: Go-to-SQL type map for the generator

Status: open
Created: 2026-11-04
Epic: momo-02-generator
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-02-generator Task 11: Go-to-SQL type map for the generator (`.procoder/plans/momo-02-generator.md:1008`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-1, S-3, S-31. Files: `gen/typemap/typemap.go`, `gen/typemap/typemap_test.go`, `gen/schema.go`. Contributes to S-1, S-3, S-31; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestGeneratorTypeMap` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

