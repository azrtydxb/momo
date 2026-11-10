# --from-db: write only the functions, policies and triggers the modeled tables use

Status: open
Created: 2026-11-10
Epic: -
Sprint: 008-sprint-8-fix-the-kuvryn-orbit-port-issues-73-to-79-and-the
Type: story

## Description

`--from-db` writes every database function into the `MomoConfig` of the alphabetically first model, so a three-table port carries about 450 lines of unrelated plpgsql, and its header hash changes on every migration. Default (integrator decision 2026-11-10): emit only the functions that a modeled table's triggers, defaults, checks or policies reference, each on the model of the table that uses it. Add `introspect.functions: all` for the old behaviour. Database-wide objects that no modeled table references are not written. GitHub issue #74.

## Acceptance criteria

- [ ] the tests are shown failing on CI before the change
- [ ] with three modeled tables, a function no modeled table references is not written; a function a modeled table's trigger calls is written on that table's model; a test pins both
- [ ] `introspect.functions: all` restores the old behaviour
- [ ] the generated file of a model does not change when an unrelated function or table changes in the database (a test regenerates after adding an unrelated function and compares)

## Evidence
