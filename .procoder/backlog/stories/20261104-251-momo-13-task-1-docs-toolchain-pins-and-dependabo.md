# momo-13 Task 1: Docs toolchain pins and Dependabot for Python

Status: open
Created: 2026-11-04
Epic: momo-13-docs-release
Sprint: -

## Description

Implements plan momo-13-docs-release Task 1: Docs toolchain pins and Dependabot for Python (`.procoder/plans/momo-13-docs-release.md:38`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-23, S-25. Files: `docs/requirements.in`, `docs/requirements.txt`, `.github/dependabot.yml`, `internal/docscheck/requirements_test.go`. Contributes to S-23, S-25; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestDocsRequirementsPinned` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

