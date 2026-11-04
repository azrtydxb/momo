# momo-09 Task 11: momolog, the slog hook

Status: open
Created: 2026-11-04
Epic: momo-09-observability-testing
Sprint: -

## Description

Implements plan momo-09-observability-testing Task 11: momolog, the slog hook (`.procoder/plans/momo-09-observability-testing.md:1962`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-87. Files: `momolog/momolog.go`, `momolog/momolog_test.go`.

## Acceptance criteria

- [ ] `TestSlogHook` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-87] `TestSlogHook` asserts log records contain SQL, duration and rows and never parameter values unless enabled; fails if params leak.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

