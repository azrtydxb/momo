# momo-11 Task 6: Type mapping for generate --from-db

Status: done 2026-11-07
Created: 2026-11-04
Epic: momo-11-adoption
Sprint: -

## Description

Implements plan momo-11-adoption Task 6: Type mapping for generate --from-db (`.procoder/plans/momo-11-adoption.md:616`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-92. Files: `gen/fromdb/mapping.go`. Contributes to S-92; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestGenerateFromDBMapping` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestGenerateFromDBUnmapped` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

CI run 42410506839 on 7ef00ed (all jobs green): TestGenerateFromDBMapping and TestGenerateFromDBUnmapped in gen/fromdb pass; issue #27.
