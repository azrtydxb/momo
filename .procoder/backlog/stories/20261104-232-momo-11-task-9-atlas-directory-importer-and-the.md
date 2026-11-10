# momo-11 Task 9: Atlas directory importer and the shared importer check

Status: open
Created: 2026-11-04
Epic: momo-11-adoption
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

Implements plan momo-11-adoption Task 9: Atlas directory importer and the shared importer check (`.procoder/plans/momo-11-adoption.md:930`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-93. Files: `importer/history/atlas/atlas.go`, `internal/importtest/importtest.go`, `internal/compat/atlasfixture/migrations/20260101000000_init.sql`.

## Acceptance criteria

- [ ] `TestHistoryImporters` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-93, part] `TestHistoryImporters` passes for the part of S-93 this task delivers; the full criterion is verified in story 235.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

