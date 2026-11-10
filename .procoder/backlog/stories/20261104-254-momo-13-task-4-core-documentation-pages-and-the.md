# momo-13 Task 4: Core documentation pages and the docs index

Status: done
Created: 2026-11-04
Epic: momo-13-docs-release
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

Implements plan momo-13-docs-release Task 4: Core documentation pages and the docs index (`.procoder/plans/momo-13-docs-release.md:460`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-21. Files: `docs/README.md`, `docs/getting-started.md`, `internal/docscheck/index_test.go`. Contributes to S-21; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestDocsIndexLinksEveryPage` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestCapabilityMatrixComplete` exists (`internal/docscheck/index_test.go:37`), fails when a capability is not documented (mutation run below), and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- red run 42454971851 (work branch suffixed -red, sha 222fa09): FAIL TestDocsIndexLinksEveryPage
- green: docs workflow run 42456103295 (sha 9b46480d8ec2f3944100975376d197a31720171c, success), where TestDocsIndexLinksEveryPage passes
- Left open: TestCapabilityMatrixComplete cannot be shown to have failed before the implementation. docs/capabilities.md already named every dialect.Dialect capability boolean, so the test passed on the red run; it is kept as a guard (it reads capabilities.md, the repository's capability page, instead of a new capability-matrix.md).
- Mutation evidence (the test cannot be shown red before its implementation, so it is shown red on a deliberate break): branch worktree-agent-ae0b59d3910467120-red, sha 89d1113, with `InsertReturning` renamed in docs/capabilities.md and the raw twin writer in tools/docs/llms_hook.py emptied: unit job and integration (postgres) job of ci-agent run 42768946200 FAIL TestCapabilityMatrixComplete (`docs/capabilities.md does not mention the dialect capability InsertReturning`); docs run 42768946297 FAIL TestLLMsTxt.
- Green: ci-agent run 42768951858 (sha 3e1eed073aed17297798962fc042066a62631848, success) and docs run 42768951678 (same sha, success). The commits carrying this work were made through the procoder commit gate.
