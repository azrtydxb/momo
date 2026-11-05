# momo-13 Task 2: MkDocs configuration, navigation and version selector

Status: open
Created: 2026-11-04
Epic: momo-13-docs-release
Sprint: -

## Description

Implements plan momo-13-docs-release Task 2: MkDocs configuration, navigation and version selector (`.procoder/plans/momo-13-docs-release.md:116`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-23. Files: `mkdocs.yml`, `internal/docscheck/nav.go`, `internal/docscheck/nav_test.go`. Contributes to S-23; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestMkdocsNavPagesExist` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestEveryDocPageInNav` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

