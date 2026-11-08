# CI gate: an exported API change needs a docs change or an explicit no-reader-visible-change line

Status: open
Created: 2026-11-08
Epic: momo-14-structure-and-docs
Sprint: -

## Description

Nothing today ties an API change to its documentation, so docs drift one merge at a time. Add a pull-request CI job that runs `golang.org/x/exp/cmd/apidiff` (pinned to a released version, run with `go run` so nothing is installed globally) between the base branch and the head for the root package and the feature subpackages, and fails when the report is non-empty unless one of two things holds: the diff touches at least one of `README.md`, `docs/**`, `llms.txt` or `examples/**` (a docs change), or the pull request body, or a commit message in the range, contains a line `Docs: no reader-visible change` followed by a reason. Both an incompatible and a compatible change count. The job comments the apidiff report on the pull request so the reviewer sees what changed. The rule for the move stories: they always touch docs, so they pass by the first branch. Use `@vN` action tags, not SHA pins, per the repository standard.

## Acceptance criteria

- [ ] a throwaway branch that adds an exported function and touches no doc fails the job (red evidence run id); the same branch with `Docs: no reader-visible change: internal helper exposed for tests` in the PR body passes
- [ ] a branch that changes only a Go comment or an unexported identifier produces an empty apidiff and passes without any line
- [ ] a branch that renames an exported function and edits `docs/ai-guide.md` passes
- [ ] the job is a required status check on main (the branch-protection change is made by the user and recorded in `.procoder/ask/decisions.md`)
- [ ] `internal/repocheck` has a test that the job exists in `.github/workflows` and runs apidiff at a pinned version (extends `TestCIWorkflowCoverage`)
- [ ] the rule and the escape-hatch line are described in `CONTRIBUTING.md`

## Evidence
