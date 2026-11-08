# momo-13 Task 4: Core documentation pages and the docs index

Status: open
Created: 2026-11-04
Epic: momo-13-docs-release
Sprint: -

## Description

Implements plan momo-13-docs-release Task 4: Core documentation pages and the docs index (`.procoder/plans/momo-13-docs-release.md:460`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-21. Files: `docs/README.md`, `docs/getting-started.md`, `internal/docscheck/index_test.go`. Contributes to S-21; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestDocsIndexLinksEveryPage` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestCapabilityMatrixComplete` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

- red run 42454971851 (work branch suffixed -red, sha 222fa09): FAIL TestDocsIndexLinksEveryPage
- green: docs workflow run 42456103295 (sha 9b46480d8ec2f3944100975376d197a31720171c, success), where TestDocsIndexLinksEveryPage passes
- Left open: TestCapabilityMatrixComplete cannot be shown to have failed before the implementation. docs/capabilities.md already named every dialect.Dialect capability boolean, so the test passed on the red run; it is kept as a guard (it reads capabilities.md, the repository's capability page, instead of a new capability-matrix.md).
