# momo-01 Task 20: Security and supply-chain workflows

Status: open
Created: 2026-11-04
Epic: momo-01-foundation
Sprint: 001-foundation-of-the-query-core-dialects-schema-base-types

## Description

Implements plan momo-01-foundation Task 20: Security and supply-chain workflows (`.procoder/plans/momo-01-foundation.md:2013`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-25, S-49. Files: `.github/workflows/codeql.yml`, `.github/dependabot.yml`, `internal/repocheck/workflows_test.go`.

## Acceptance criteria

- [ ] `TestWorkflowsPinned` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestCodeQLAndGosecRunOnPullRequests` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-25] `TestWorkflowsPinned` parses .github/workflows and asserts every third-party `uses:` is pinned by a 40-character SHA and every workflow declares `permissions:`; CI runs govulncheck and the Scorecard workflow; fails if an action is referenced by tag.
- [x] [S-49] `TestWorkflowsPinned` also asserts CodeQL and gosec workflows exist and run on pull requests; fails if either is absent.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- `TestCodeQLAndGosecRunOnPullRequests`: before the workflow files existed, `go test ./internal/repocheck/ -run 'TestWorkflowsPinned|TestCodeQLAndGosecRunOnPullRequests' -count=1` failed with `open .../codeql.yml: no such file or directory`; after: `ok`.
- `TestWorkflowsPinned` did NOT fail before the implementation: it only walks existing workflows, and ci.yml (Task 19) was already pinned, so it passed vacuously. Its first criterion stays unchecked. It passes now over ci.yml, codeql.yml, gosec.yml and scorecard.yml (every third-party `uses:` is a 40-hex SHA, every workflow has top-level `permissions`).
- [S-25]: govulncheck job in ci.yml (asserted by `TestCIWorkflowCoverage`), scorecard.yml runs on schedule (asserted by `TestCodeQLAndGosecRunOnPullRequests`); dependabot.yml exists with gomod and github-actions weekly.
- [S-49]: codeql.yml and gosec.yml exist and run on pull_request (asserted by `TestCodeQLAndGosecRunOnPullRequests`; that test, not `TestWorkflowsPinned`, holds this assertion).
- `go test ./internal/repocheck/ -count=1 -v`: all five tests PASS. `actionlint` over all workflows: exit 0, no output.
- Gate: `go build ./... && go vet ./... && go test -race ./...` all `ok`; `launcher.sh check`: `procoder gate: 5 clean, 0 unformatted, 0 unchecked, 0 out of scope, 20 hygiene finding(s) (0 blocking)`; commit 70f10bd.

