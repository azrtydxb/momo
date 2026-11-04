# momo-01 Task 20: Security and supply-chain workflows

Status: open
Created: 2026-11-04
Epic: momo-01-foundation
Sprint: -

## Description

Implements plan momo-01-foundation Task 20: Security and supply-chain workflows (`.procoder/plans/momo-01-foundation.md:2013`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-25, S-49. Files: `.github/workflows/codeql.yml`, `.github/dependabot.yml`, `internal/repocheck/workflows_test.go`.

## Acceptance criteria

- [ ] `TestWorkflowsPinned` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestCodeQLAndGosecRunOnPullRequests` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-25] `TestWorkflowsPinned` parses .github/workflows and asserts every third-party `uses:` is pinned by a 40-character SHA and every workflow declares `permissions:`; CI runs govulncheck and the Scorecard workflow; fails if an action is referenced by tag.
- [ ] [S-49] `TestWorkflowsPinned` also asserts CodeQL and gosec workflows exist and run on pull requests; fails if either is absent.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

