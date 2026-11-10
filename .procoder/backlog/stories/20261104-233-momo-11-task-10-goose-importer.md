# momo-11 Task 10: goose importer

Status: open
Created: 2026-11-04
Epic: momo-11-adoption
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

Implements plan momo-11-adoption Task 10: goose importer (`.procoder/plans/momo-11-adoption.md:1125`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-93. Files: `importer/history/goose/goose.go`, `internal/compat/goosefixture/migrations/00001_init.sql`.

## Acceptance criteria

- [ ] `TestHistoryImporters` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-93, part] `TestHistoryImporters` passes for the part of S-93 this task delivers; the full criterion is verified in story 235.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- importer/history/goose/goose.go, fixtures internal/compat/goosefixture/migrations.
- `TestHistoryImporters/goose` (integration tag) passes in the integration (postgres) job of ci-agent run 42774384580, headSha 214cb28dc323466eca185bd46e7079ac8f4f6e08, all 9 jobs success.
- Commit f567583 went through the procoder gate.
- Red-first not evidenced: the implementation was written before its test ran and no failing CI run was recorded, so the 'failed before the implementation' criterion stays unticked (the tests pass).
