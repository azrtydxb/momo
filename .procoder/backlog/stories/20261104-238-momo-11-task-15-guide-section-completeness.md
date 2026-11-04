# momo-11 Task 15: Guide section completeness

Status: open
Created: 2026-11-04
Epic: momo-11-adoption
Sprint: -

## Description

Implements plan momo-11-adoption Task 15: Guide section completeness (`.procoder/plans/momo-11-adoption.md:1585`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-95. Files: `internal/docscheck/guides_test.go`.

## Acceptance criteria

- [ ] `TestMigrationGuidesComplete` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-95] `TestDocsCodeBlocks` covers the guide pages and `TestMigrationGuidesComplete` asserts each guide has sections for models, queries, relations, hooks, transactions, migrations and errors; fails if a guide section is missing.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

