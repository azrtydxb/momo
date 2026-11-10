# momo-12 Task 11: Reproducible `make bench`, results layout and docs/PERFORMANCE.md

Status: open
Created: 2026-11-04
Epic: momo-12-performance
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

Implements plan momo-12-performance Task 11: Reproducible `make bench`, results layout and docs/PERFORMANCE.md (`.procoder/plans/momo-12-performance.md:1411`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-19. Files: `Makefile`, `bench/scripts/run.sh`, `bench/results/README.txt`, `bench/scripts/run_test.go`, `docs/PERFORMANCE.md`. Contributes to S-19; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestBenchResultsLayout` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- TestBenchResultsLayout: bench/scripts/run_test.go runs run.sh smoke and checks env.json, versions.txt, raw.txt and summary.txt. make bench, make bench-suite-smoke, make bench-baseline and make bench-generate exist; docs/PERFORMANCE.md holds the methodology, pinned versions, reproduction commands and regression policy.
  Red proof: ci-agent run 42465164717 (headSha 0c7f14e) on branch worktree-agent-a51f3a530eba5c020-adapters-red, which keeps the tests and removes the implementations: bench-module fails with `no required module provides package` for the removed adapter packages and unit fails with `momo.InsertQuery[suite.Document] has no field or method Unnest`.
  Green proof: ci-agent run 42465734475, headSha eaec549861a73104452a68df580ea15421eac7fb, all eight jobs green (lint, unit, fuzz-smoke, integration postgres, integration mysql84/mysql97/mariadb, bench-module).
- Not done: plan Step 3, a committed make bench run on a dedicated machine, and with it any published comparative budget in docs/PERFORMANCE.md. This environment has no dedicated machine, and numbers from a shared runner would contradict the page's own policy. docs/PERFORMANCE.md says so.
  Gate: commits went through the procoder commit hook, with a hand-written go.work (untracked) so the hook can lint the nested bench module.
