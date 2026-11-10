# momo-11 Task 5: `momo import gorm|bun|ent` command

Status: open
Created: 2026-11-04
Epic: momo-11-adoption
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

Implements plan momo-11-adoption Task 5: `momo import gorm|bun|ent` command (`.procoder/plans/momo-11-adoption.md:547`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-91. Files: `cmd/momo/import.go`, `cmd/momo/main.go`. Contributes to S-91; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestImportCommand` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestImportRefusesUnsupported` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

