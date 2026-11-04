# momo-11 Task 8: History import framework

Status: open
Created: 2026-11-04
Epic: momo-11-adoption
Sprint: -

## Description

Implements plan momo-11-adoption Task 8: History import framework (`.procoder/plans/momo-11-adoption.md:799`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-93. Files: `importer/history/history.go`.

## Acceptance criteria

- [ ] `TestConvertKeepsTimestampsAndMapsOrdinals` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestDirtySourceRefused` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-93] `TestHistoryImporters` imports each fixture history, asserts status shows all versions applied and drift is zero; fails if an applied migration would re-run.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

