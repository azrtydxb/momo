# momo-11 Task 15: Guide section completeness

Status: open
Created: 2026-11-04
Epic: momo-11-adoption
Sprint: -

## Description

Implements plan momo-11-adoption Task 15: Guide section completeness (`.procoder/plans/momo-11-adoption.md:1585`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-95. Files: `internal/docscheck/guides_test.go`.

## Acceptance criteria

- [ ] `TestMigrationGuidesComplete` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-95, part] `TestMigrationGuidesComplete` passes for the part of S-95 this task delivers; the full criterion is verified in story 255.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

