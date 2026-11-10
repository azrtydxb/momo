# momo-11 Task 12: Bun migration history importer

Status: open
Created: 2026-11-04
Epic: momo-11-adoption
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

Implements plan momo-11-adoption Task 12: Bun migration history importer (`.procoder/plans/momo-11-adoption.md:1349`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-93. Files: `importer/history/bun/bun.go`, `internal/compat/bunmigfixture/migrations/20260101000000_init.up.sql`.

## Acceptance criteria

- [ ] `TestHistoryImporters` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-93] `TestHistoryImporters` imports each fixture history, asserts status shows all versions applied and drift is zero; fails if an applied migration would re-run.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

