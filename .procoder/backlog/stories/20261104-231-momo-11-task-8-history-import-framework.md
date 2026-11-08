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

## Deviations from the plan

- `Convert` also refuses a source with unapplied migrations (`ErrSourcePending`; nil `Applied` means the reader does not know, an empty non-nil one means none applied) and a result whose versions would not ascend strictly (mapped version after a timestamp version).
- Mapped versions are `MappedBase` plus the 1-based list position in seconds, because `...000060` is not a valid timestamp.
- `HistoryRows` returns an error and fails on a nil `momo.sum` or a missing entry, so a row can never carry an empty checksum.
- `WriteImport` validates version and name first and writes into a temporary directory renamed into place; the target must not exist or must be empty.
- `Baseline` refuses a scratch database that is not empty.

## Open items for the live writer (later task)

The code that writes the history rows into a real database is not part of this story. It must hold the migration lock, insert all rows in one transaction, and be idempotent (ON CONFLICT DO NOTHING), so a crash or a repeat run cannot leave a half-imported history.
