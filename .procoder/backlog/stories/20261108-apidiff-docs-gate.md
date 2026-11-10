# CI gate: an exported API change needs a docs change or an explicit no-reader-visible-change line

Status: in-progress
Created: 2026-11-08
Epic: momo-14-structure-and-docs
Sprint: 006-sprint-6-split-the-root-package-into-subpackages-and-keep

## Description

Nothing today ties an API change to its documentation, so docs drift one merge at a time. Add a pull-request CI job that runs `golang.org/x/exp/cmd/apidiff` (pinned to a released version, run with `go run` so nothing is installed globally) between the base branch and the head for the root package and the feature subpackages, and fails when the report is non-empty unless one of two things holds: the diff touches at least one of `README.md`, `docs/**`, `llms.txt` or `examples/**` (a docs change), or the pull request body, or a commit message in the range, contains a line `Docs: no reader-visible change` followed by a reason. Both an incompatible and a compatible change count. The job comments the apidiff report on the pull request so the reviewer sees what changed. The rule for the move stories: they always touch docs, so they pass by the first branch. Use `@vN` action tags, not SHA pins, per the repository standard.

## Acceptance criteria

- [x] a throwaway branch that adds an exported function and touches no doc fails the job (red evidence run id); the same branch with `Docs: no reader-visible change: internal helper exposed for tests` in the PR body passes
- [x] a branch that changes only a Go comment or an unexported identifier produces an empty apidiff and passes without any line
- [x] a branch that renames an exported function and edits `docs/ai-guide.md` passes
- [ ] the job is a required status check on main (the branch-protection change is made by the user and recorded in `.procoder/ask/decisions.md`)
- [x] `internal/repocheck` has a test that the job exists in `.github/workflows` and runs apidiff at a pinned version (extends `TestCIWorkflowCoverage`)
- [x] the rule and the escape-hatch line are described in `CONTRIBUTING.md`

## Evidence

Run evidence (ci-agent, 2026-11-08, job `apidiff-docs`):

- Criterion 1, red half: run 42494457434 (probe-a, 02cd03f) adds exported `momolog.Probe` and no docs; `apidiff-docs` failed with `./momolog.Probe: added` and "Docs gate failed". Escape half: run 42494459685 (probe-b, 0394aff) adds a commit-message escape line; `apidiff-docs` passed. Left unticked: the PR-body variant cannot run on ci-agent (no pull request); only the commit-message path is proven.
- Criterion 2: run 42494469285 (probe-c, 66765de) changes a comment and adds an unexported function; `apidiff-docs` passed with no line.
- Criterion 3: run 42494497016 (probe-d, 7232849) renames `momolog.Values` to `LogValues` and edits `docs/ai-guide.md`; `apidiff-docs` passed.
- Criterion 4: not done. Needs the user's branch-protection change and a decisions.md entry.
- Criterion 5: `TestAPIDiffDocsGate` in `internal/repocheck/apidiff_gate_test.go`. Red: run 42494372028 (0fe79a8, test only) failed `unit`. Green: run 42494433597 (f0b30ce) all jobs passed.
- Criterion 6: CONTRIBUTING.md section "Exported API changes need a docs change" (f0b30ce).

Review round 2026-11-09 (fixes in 4a0df53, ci-agent job `apidiff-docs`; the required-check criterion stays unticked, the user sets branch protection after the sprint-6 to main PR):

- Body edits: the gate moved to `.github/workflows/apidiff-docs.yml` (types opened, synchronize, reopened, edited; job id `apidiff-docs`), which reads the live body with `gh pr view`; ci.yml no longer defines the job, so the ci.yml matrix does not re-run on a body edit. The pull_request path itself cannot run on ci-agent and is first exercised on the sprint-6 to main PR.
- Escape line must start in column 0 with a real reason: run 42575395073 (933255e) has a `<reason>` placeholder line and an indented line; `apidiff-docs` failed.
- Docs match limited to README.md, llms.txt, docs/**/*.md, examples/**/README.md: run 42575386754 (84ca00e) adds an exported function and changes docs/requirements.txt only; `apidiff-docs` failed.
- Package moves: run 42575926402 (d22b8c4) deletes exported root func `WithMaxInList`, adds package `foo/` and an exported func in `internal/probe`, no docs; `apidiff-docs` failed reporting `WithMaxInList: removed` (incompatible) and `package github.com/azrtydxb/momo/foo: added`, and `internal/probe` was skipped ("Ignoring internal package"). An earlier probe (run 42575378098) deleted `WithClock`, still used by internal/suite, and failed on a type error, so it proves nothing.
- Comment: one comment marked `<!-- apidiff-docs -->` is edited in place, none is posted when the API did not change; the report is written under RUNNER_TEMP and `/apidiff-report.md` is git-ignored. Not exercisable on ci-agent.

PR-body path (2026-11-09, workflow `apidiff-docs.yml`, throwaway draft PR #51 `apidiff-probe-prbody` into sprint-6, head 55ca940, adds exported `momolog.ProbePRBody` and no docs):

- Red: run 42668537688 (pull_request, opened) failed with `./momolog.ProbePRBody: added` and "Docs gate failed: the exported API changed but no docs changed."
- Escape: after adding `Docs: no reader-visible change: internal helper exposed for tests` to the PR body, run 42668814473 (pull_request, edited) passed with "Docs gate passed by the escape line". Criterion 1 is ticked on these two runs. PR #51 closed unmerged and its branch deleted.

