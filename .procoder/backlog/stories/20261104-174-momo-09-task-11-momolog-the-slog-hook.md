# momo-09 Task 11: momolog, the slog hook

Status: done 2026-11-07
Created: 2026-11-04
Epic: momo-09-observability-testing
Sprint: -

## Description

Implements plan momo-09-observability-testing Task 11: momolog, the slog hook (`.procoder/plans/momo-09-observability-testing.md:1962`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-87. Files: `momolog/momolog.go`, `momolog/momolog_test.go`.

## Acceptance criteria

- [x] `TestSlogHook` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-87] `TestSlogHook` asserts log records contain SQL, duration and rows and never parameter values unless enabled; fails if params leak.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- `TestSlogHook`: red run 42412075743 (3393482, vet: undefined momolog.Option), green run 42414180063 (72bd53a).
- Mutation: logging args without the Values option made `TestSlogHook` fail with `argument value leaked` (run 42415019735, reverted).
