# momo-11 Task 9: Atlas directory importer and the shared importer check

Status: open
Created: 2026-11-04
Epic: momo-11-adoption
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

Implements plan momo-11-adoption Task 9: Atlas directory importer and the shared importer check (`.procoder/plans/momo-11-adoption.md:930`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-93. Files: `importer/history/atlas/atlas.go`, `internal/importtest/importtest.go`, `internal/compat/atlasfixture/migrations/20260101000000_init.sql`.

## Acceptance criteria

- [ ] `TestHistoryImporters` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-93, part] `TestHistoryImporters` passes for the part of S-93 this task delivers; the full criterion is verified in story 235.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- importer/history/atlas/atlas.go, internal/importtest/importtest.go, fixtures internal/compat/atlasfixture/migrations (atlas.sum computed with the Atlas HashFile algorithm read at ariga.io/atlas sql/migrate/dir.go:662 and :693, not with the Atlas CLI).
- `TestHistoryImporters/atlas` (integration tag) passes in the integration (postgres) job of ci-agent run 42780875746, headSha 981b3c0777de48b0ac7870ec9e48304aa2dae545, all 9 jobs success.
- Commit f567583 went through the procoder gate.
- Red-first not evidenced: the implementation was written before its test ran and no failing CI run was recorded, so the 'failed before the implementation' criterion stays unticked (the tests pass).
- Review fix: Read now errors when an applied revision has no file (TestReadRefusesAppliedRevisionWithoutFile) and refuses duplicate numeric versions.
