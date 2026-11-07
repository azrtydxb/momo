# momo-05 Task 32: Named scopes, typed fragments and runtime filter tree (S-79)

Status: open
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: -

## Description

Implements plan momo-05-writes-queries Task 32: Named scopes, typed fragments and runtime filter tree (S-79) (`.procoder/plans/momo-05-writes-queries.md:2633`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-79. Files: `compose.go`, `compose_test.go`.

## Acceptance criteria

- [ ] `TestCompositionHelpers` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-79] `TestCompositionHelpers` asserts each renders correctly or returns ErrUnsupported per dialect and the filter tree rejects unknown columns; fails if an override bypasses identifier quoting.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

- CI run 42410267553 (ci-agent, commit d310d41) green: unit, lint, fuzz-smoke and integration (postgres) jobs pass with this story's tests.
- Criteria left unticked: the red-first claim (no pre-implementation run exists; the first CI run only failed on two fixture mistakes fixed in d310d41) and the local procoder check gate (not run on this laptop).
