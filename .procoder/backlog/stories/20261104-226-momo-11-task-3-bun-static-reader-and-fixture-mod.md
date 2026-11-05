# momo-11 Task 3: Bun static reader and fixture module

Status: open
Created: 2026-11-04
Epic: momo-11-adoption
Sprint: -

## Description

Implements plan momo-11-adoption Task 3: Bun static reader and fixture module (`.procoder/plans/momo-11-adoption.md:267`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-91. Files: `importer/bun/read.go`.

## Acceptance criteria

- [ ] `TestImportBun` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestReadReportsNullzeroWithPosition` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-91, part] `TestImportBun` passes for the part of S-91 this task delivers; the full criterion is verified in story 227.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

