# momo-05 Task 21: Go-side defaults and validators before SQL (S-72)

Status: open
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: -

## Description

Implements plan momo-05-writes-queries Task 21: Go-side defaults and validators before SQL (S-72) (`.procoder/plans/momo-05-writes-queries.md:1714`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-72. Files: `fields.go`, `fields_test.go`, `meta.go`.

## Acceptance criteria

- [ ] `TestDefaultsAndValidators` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-72] `TestDefaultsAndValidators` asserts defaults apply only to omitted fields, validators reject invalid values with a structured validation error before any SQL runs (statement-count hook = 0); fails if validation runs after execution.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

