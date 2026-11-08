# momo-13 Task 3: Runnable examples, run through testopen.Each

Status: open
Created: 2026-11-04
Epic: momo-13-docs-release
Sprint: -

## Description

Implements plan momo-13-docs-release Task 3: Runnable examples, run through testopen.Each (`.procoder/plans/momo-13-docs-release.md:275`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-21. Files: `examples/momo.yaml`, `examples/migrations/embed.go`, `examples/internal/open/open.go`, `examples/crud/example.go`, `examples/examples_test.go`.

## Acceptance criteria

- [ ] `TestExamples` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-21] `TestExamples` runs every example end to end against both databases in CI; docs/ contains architecture, capability matrix, migration safety and recovery guide, contributor guide and supported versions; fails if an example is not wired into the test.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

- Left open: this repository runs its examples through per-example tests built on examples/internal/exampletest, not a single TestExamples over testopen.Each, and only the PostgreSQL entry exists. TestExamplesAreWired (internal/repocheck) already fails when an example has no test. No code was changed here.
