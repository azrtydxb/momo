# momo-05 Task 21: Go-side defaults and validators before SQL (S-72)

Status: open
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-05-writes-queries Task 21: Go-side defaults and validators before SQL (S-72) (`.procoder/plans/momo-05-writes-queries.md:1714`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-72. Files: `fields.go`, `fields_test.go`, `meta.go`.

## Acceptance criteria

- [x] `TestDefaultsAndValidators` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-72] `TestDefaultsAndValidators` asserts defaults apply only to omitted fields, validators reject invalid values with a structured validation error before any SQL runs (statement-count hook = 0); fails if validation runs after execution.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

- Red first: `go vet .` failed with `undefined: momo.FieldHook` before `fields.go` existed (test files written first).
- `TestDefaultsAndValidators` (`fields_test.go`) passes on PostgreSQL with `-tags integration -race`. Deviation recorded in the plan (Task 21 reconciliation note, 2026-11-05): the plan test is self-contradictory (two zero-version `Model` inserts expecting 1 and 0), so the default case uses `.Omit(suite.DocVersion)`.
- S-72: the test asserts the default applies only to an omitted column and an explicit zero is kept, an empty title fails with `ErrValidation`, and a `WithHook` statement counter is 0 after the failure. `fields_unit_test.go` (fake driver, 0 recorded calls) covers Values, ValuesRows, Model, Models, update, Model update, upsert and DoUpdate values, plus a chunked bulk insert that validates all rows before the first chunk. `fields_integration_test.go` (`TestFieldHooksLive`) proves the Go default beats the DB default, OnUpdate, and that rejected writes leave the table untouched, chunked included.
- Mutation check (all killed by the tests): skip model validation, skip bulk pre-validation, apply default to a non-omitted Model column, apply default over an assigned Values column, skip update validation, drop the explicit-Set-wins rule, skip DoUpdate validation.
- `make fmt-check vet staticcheck gosec tidy-check` clean. Generator emission of `Meta.Fields` is not part of this task, so no goldens changed. SQLite and MySQL runs remain with momo-03.
