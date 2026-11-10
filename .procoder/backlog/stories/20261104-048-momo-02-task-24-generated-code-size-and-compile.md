# momo-02 Task 24: Generated-code size and compile-time measurement

Status: open
Created: 2026-11-04
Epic: momo-02-generator
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each
Carried: 002-generator-from-go-structs-with-its-type-prerequisites — Not implemented: internal/tools/genstats, bench/generated-code/README.md and TestGenStatsOutput do not exist in the repo; both criteria unmet.

## Description

Implements plan momo-02-generator Task 24: Generated-code size and compile-time measurement (`.procoder/plans/momo-02-generator.md:2635`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-19. Files: `internal/tools/genstats/main.go`, `internal/tools/genstats/genstats_test.go`, `bench/generated-code/README.md`, `bench/results/generated-code/.gitkeep`. Contributes to S-19; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestGenStatsOutput` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

