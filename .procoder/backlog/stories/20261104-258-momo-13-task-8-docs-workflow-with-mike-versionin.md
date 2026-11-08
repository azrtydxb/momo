# momo-13 Task 8: Docs workflow with mike versioning

Status: open
Created: 2026-11-04
Epic: momo-13-docs-release
Sprint: -

## Description

Implements plan momo-13-docs-release Task 8: Docs workflow with mike versioning (`.procoder/plans/momo-13-docs-release.md:900`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-23, S-25. Files: `.github/workflows/docs.yml`, `internal/repocheck/docs_workflow_test.go`. Contributes to S-23, S-25; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestDocsWorkflow` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- red run 42454971851 (work branch suffixed -red, sha 222fa09, tests with a NavPages stub): FAIL TestDocsWorkflow
- green: docs workflow run 42456103295 (sha 9b46480d8ec2f3944100975376d197a31720171c, success) and the unit and integration jobs of ci-agent run 42456103287 on the same sha, where TestDocsWorkflow passes
- docs.yml: the build job runs on pull_request, push to main, to sprint branches and to worktree-agent branches, and on version tags; the deploy job (mike deploy --push dev on main; versioned release plus latest on version tags) is gated by an if on main and tags only and is the only job with contents: write. The first real deploy can only be observed after this workflow reaches main.
