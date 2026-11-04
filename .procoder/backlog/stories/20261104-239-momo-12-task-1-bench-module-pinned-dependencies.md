# momo-12 Task 1: bench module, pinned dependencies and environment capture

Status: open
Created: 2026-11-04
Epic: momo-12-performance
Sprint: -

## Description

Implements plan momo-12-performance Task 1: bench module, pinned dependencies and environment capture (`.procoder/plans/momo-12-performance.md:53`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-19. Files: `bench/go.mod`, `bench/env/env.go`, `bench/cmd/envcapture/main.go`. Contributes to S-19; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestEnvCapture` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

