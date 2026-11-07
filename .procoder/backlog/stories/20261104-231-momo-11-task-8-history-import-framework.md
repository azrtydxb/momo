# momo-11 Task 8: History import framework

Status: done 2026-11-07
Created: 2026-11-04
Epic: momo-11-adoption
Sprint: -

## Description

Implements plan momo-11-adoption Task 8: History import framework (`.procoder/plans/momo-11-adoption.md:799`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-93. Files: `importer/history/history.go`. Contributes to S-93; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestConvertKeepsTimestampsAndMapsOrdinals` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestDirtySourceRefused` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

TestConvertKeepsTimestampsAndMapsOrdinals and TestDirtySourceRefused in importer/history/history_test.go; Baseline covered by baseline_integration_test.go; CI run 42411250657 (sha 946a2a6) success.
