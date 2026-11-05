# momo-13 Task 8: Docs workflow with mike versioning

Status: open
Created: 2026-11-04
Epic: momo-13-docs-release
Sprint: -

## Description

Implements plan momo-13-docs-release Task 8: Docs workflow with mike versioning (`.procoder/plans/momo-13-docs-release.md:900`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-23, S-25. Files: `.github/workflows/docs.yml`, `internal/repocheck/docs_workflow_test.go`. Contributes to S-23, S-25; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestDocsWorkflow` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

