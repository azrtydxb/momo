# momo-11 Task 4: Ent static reader and fixture module (no code execution)

Status: open
Created: 2026-11-04
Epic: momo-11-adoption
Sprint: -

## Description

Implements plan momo-11-adoption Task 4: Ent static reader and fixture module (no code execution) (`.procoder/plans/momo-11-adoption.md:396`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-91. Files: `importer/ent/read.go`.

## Acceptance criteria

- [ ] `TestImportEnt` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestReadUnsupportedBuilderIsDiagnosed` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-91] `TestImportGORM`, `TestImportBun`, `TestImportEnt` convert fixture projects, assert generated Momo models produce the same schema as the source ORM's migration (inspected) with zero diff, and assert the report lists each semantic change; fails if a source field is dropped silently.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

