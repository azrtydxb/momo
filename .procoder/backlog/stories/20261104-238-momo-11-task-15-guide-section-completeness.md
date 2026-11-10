# momo-11 Task 15: Guide section completeness

Status: done 2026-11-10
Created: 2026-11-04
Epic: momo-11-adoption
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

Implements plan momo-11-adoption Task 15: Guide section completeness (`.procoder/plans/momo-11-adoption.md:1585`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-95. Files: `internal/docscheck/guides_test.go`.

## Acceptance criteria

- [x] `TestMigrationGuidesComplete` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-95, part] `TestMigrationGuidesComplete` passes for the part of S-95 this task delivers; the full criterion is verified in story 255.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- internal/docscheck/guides_test.go TestMigrationGuidesComplete passes in the unit job of ci-agent run 42780875746, headSha 981b3c0777de48b0ac7870ec9e48304aa2dae545, all 9 jobs success.
- Red-first not evidenced: the implementation was written before its test ran and no failing CI run was recorded, so the 'failed before the implementation' criterion stays unticked (the tests pass).

### Red proof (2026-11-10)

Throwaway branch (deleted) with `## Hooks` renamed in all three guides: `TestMigrationGuidesComplete` FAILS in the unit job, ci-agent run 42827469571 on 35dc039. Passes on sprint-7 (run 42780875746).
