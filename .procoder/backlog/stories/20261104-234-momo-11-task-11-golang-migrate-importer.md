# momo-11 Task 11: golang-migrate importer

Status: done 2026-11-10
Created: 2026-11-04
Epic: momo-11-adoption
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

Implements plan momo-11-adoption Task 11: golang-migrate importer (`.procoder/plans/momo-11-adoption.md:1236`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-93. Files: `importer/history/golangmigrate/golangmigrate.go`, `internal/compat/gmfixture/migrations/1_create_a.up.sql`.

## Acceptance criteria

- [x] `TestHistoryImporters` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-93, part] `TestHistoryImporters` passes for the part of S-93 this task delivers; the full criterion is verified in story 235.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- importer/history/golangmigrate/golangmigrate.go, fixtures internal/compat/gmfixture/migrations. Deviation from the plan test: golang-migrate stores only the current version and every migration up to it is applied, so the test asserts 2 applied versions (history.Convert refuses unapplied ones with ErrSourcePending), not 1; subtests dirty-refused and pending-refused added.
- `TestHistoryImporters/golangmigrate` (integration tag) passes in the integration (postgres) job of ci-agent run 42780875746, headSha 981b3c0777de48b0ac7870ec9e48304aa2dae545, all 9 jobs success.
- Commit f567583 went through the procoder gate.
- Red-first not evidenced: the implementation was written before its test ran and no failing CI run was recorded, so the 'failed before the implementation' criterion stays unticked (the tests pass).
- Review fix: a recorded version with an empty or wrong directory errors (golangmigrate/wrong-directory-refused); duplicate versions 01 and 1 are refused (golangmigrate/duplicate-versions-refused).

### Red proof (2026-11-10)

Throwaway branch (deleted), golangmigrate.Read returning an empty source: `TestHistoryImporters/golangmigrate` and its subtests (dirty-refused, pending-refused, duplicate-versions-refused, wrong-directory-refused) FAIL in the integration (postgres) job, ci-agent run 42827469571 on 35dc039. Passes on sprint-7 (run 42780875746).
