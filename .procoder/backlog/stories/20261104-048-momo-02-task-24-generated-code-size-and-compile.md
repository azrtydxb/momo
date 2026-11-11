# momo-02 Task 24: Generated-code size and compile-time measurement

Status: done 2026-11-11
Created: 2026-11-04
Epic: momo-02-generator
Sprint: 009-fix-every-open-github-issue-86-to-108-the-kuvryn-orbit-port
Carried: 007-sprint-7-finish-every-open-backlog-story-auditing-each — Superseded by bench/codesize (Measure, TestCodeSizeReport, bench/cmd/codesize); the plan's internal/tools/genstats, TestGenStatsOutput and bench/generated-code/README.md were not built and the plan's package layout is not buildable. Needs a user decision to retire or reword the criteria; not weakened here.
Carried: 002-generator-from-go-structs-with-its-type-prerequisites — Not implemented: internal/tools/genstats, bench/generated-code/README.md and TestGenStatsOutput do not exist in the repo; both criteria unmet.

## Description

Implements plan momo-02-generator Task 24: Generated-code size and compile-time measurement (`.procoder/plans/momo-02-generator.md:2635`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-19. Files: `internal/tools/genstats/main.go`, `internal/tools/genstats/genstats_test.go`, `bench/generated-code/README.md`, `bench/results/generated-code/.gitkeep`. Contributes to S-19; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] [S-19] `TestCodeSizeReport` (bench/codesize/codesize_test.go:13) measures the generated model package with `bench/codesize.Measure` (command `bench/cmd/codesize`): generated files, bytes and lines are non-zero, the model package compile time is measured, and every generated file keeps its DO NOT EDIT header; it passes in the bench-module job.
- [x] `procoder check` gate is clean and the task's commit is made (the task's code is on main at 4781864; the lint job 130341061844 of ci.yml run 42965315635 is success; the commit gate ran on the sprint 9 bookkeeping commit)

## Evidence

Carry reason: superseded by a different design that shipped. Generated-code size and compile cost (the S-19 half this task delivers) is measured by `bench/codesize` (`Measure` at bench/codesize/codesize.go:37 reports generated files, bytes, lines and the median package compile time) with the command in `bench/cmd/codesize/main.go:16`, and `TestCodeSizeReport` (bench/codesize/codesize_test.go:13) pins the report. The plan's `internal/tools/genstats`, `TestGenStatsOutput` and `bench/generated-code/README.md` would duplicate it with a second JSON schema and do not fit the bench module layout (the plan's `package genstats` test next to a `main` is also not buildable as written). Criteria left unticked for the integrator to carry.

## Reworded 2026-11-11 (user decision)

User decision of 2026-11-11, recorded in .procoder/ask/decisions.md, section "Carried plan stories": reword the criteria to the design that shipped, cite CI evidence, and close the story. The `Carried:` lines above stay as the history.

- Original: `TestGenStatsOutput` exists as written in the plan, failed before the implementation, and passes now
  - Reason: the plan's `internal/tools/genstats`, `TestGenStatsOutput` and `bench/generated-code/README.md` were never built. Generated-code size and compile time (the S-19 half of this task) are delivered by `bench/codesize` and pinned by `TestCodeSizeReport`. Building the plan's package too would duplicate it with a second JSON schema, and its `package genstats` test next to a `main` is not buildable as written.

### Sprint 9 CI evidence (2026-11-11)

Jobs of ci.yml run 42965315635 on main (headSha 47818647165f0231b1117bb6ebe2bce67dbed444, conclusion success). The job logs carry package-level `ok` lines, not `-v` output: unit (1.26) 130341061373 (`make test`), race (1.26) 130341061707 (`make test-race`), integration (1.27, 0.8.7-pg17, postgres) 130341061675 (`make test-integration-postgres`: MOMO_REQUIRE_INTEGRATION=1 and `-tags integration -race`, so a skipped database test fails), windows 130341061640, bench-module 130341061885 (`make test-bench-module`), lint 130341061844.

- `TestCodeSizeReport` exists at bench/codesize/codesize_test.go:13 (grep). Its package ran in the bench-module job 130341061885: `ok github.com/azrtydxb/momo/bench/codesize`.
