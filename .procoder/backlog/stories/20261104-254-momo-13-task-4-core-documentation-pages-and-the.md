# momo-13 Task 4: Core documentation pages and the docs index

Status: open
Created: 2026-11-04
Epic: momo-13-docs-release
Sprint: -

## Description

Implements plan momo-13-docs-release Task 4: Core documentation pages and the docs index (`.procoder/plans/momo-13-docs-release.md:460`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-21. Files: `docs/README.md`, `docs/getting-started.md`, `internal/docscheck/index_test.go`.

## Acceptance criteria

- [ ] `TestDocsIndexLinksEveryPage` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestCapabilityMatrixComplete` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-21] `TestExamples` runs every example end to end against both databases in CI; docs/ contains architecture, capability matrix, migration safety and recovery guide, contributor guide and supported versions; fails if an example is not wired into the test.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

