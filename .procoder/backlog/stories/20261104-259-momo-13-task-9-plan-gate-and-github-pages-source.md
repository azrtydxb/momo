# momo-13 Task 9: Plan gate and GitHub Pages source

Status: done 2026-11-10
Created: 2026-11-04
Epic: momo-13-docs-release
Sprint: -
Carried: 007-sprint-7-finish-every-open-backlog-story-auditing-each — Step 5 needs /llms.txt at the site root. The root copy (decision 2026-11-10) is implemented in 2a3addc and runs on release tags only, so it is observable at the next release.

## Description

Implements plan momo-13-docs-release Task 9: Plan gate and GitHub Pages source (`.procoder/plans/momo-13-docs-release.md:985`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-21, S-23. Contributes to S-21, S-23; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] Step 1: Run the Go gate. `make fmt-check vet test` and `go test ./internal/docscheck/ ./internal/repocheck/ -count=1`. Expected: exit 0 for each.
- [x] Step 2: Run the example and block gate against the PostgreSQL service. `make db-up`, then `make test-integration-postgres` (the examples and the runnable blocks run through `testopen.Each`). Expected: `ok`. Then `make db-down`.
- [x] Step 3: Build the site as CI does. With the venv from Task 6: `mkdocs build --strict --site-dir "$SCRATCH/site"` exits 0, then `MOMO_DOCS_SITE="$SCRATCH/site" go test -tags docs -count=1 -run '^TestLLMsTxt$' ./internal/docscheck/` prints `ok`.
- [x] Step 4: Set the Pages source after the first deploy. The `gh-pages` branch is created by the first `mike deploy --push dev` on main (Task 8), so run this step after that deploy succeeds. Requires repository administration rights.
- [x] Step 5: Verify the Pages source and the published files. `gh api repos/azrtydxb/momo/pages --jq '.source.branch, .html_url'` must print `gh-pages` and `https://azrtydxb.github.io/momo/`. Then `curl -fsI https://azrtydxb.github.io/momo/llms.txt` must report `HTTP/2 200` (retry for up to ten minutes…
- [x] `procoder check` gate is clean

## Evidence

- Steps 1 to 3 are covered by the docs workflow, run 42456103295: strict mkdocs build, TestLLMsTxt, code blocks and runnable blocks.
- Left open: steps 4 and 5 (set the Pages source on gh-pages, check the published llms.txt) need the first mike deploy from main and repository administration rights; they cannot run before docs.yml is on main.
- Steps 1 and 2: ci-agent run 42768951858 (sha 3e1eed0, success): unit, apidiff-docs and the integration jobs. Step 3: docs run 42768951678 (same sha, success) runs the strict mkdocs build and TestLLMsTxt.
- Step 4: gh-pages existed (`gh api repos/azrtydxb/momo/branches/gh-pages --jq .name` printed gh-pages); Pages did not (GET returned 404), so `gh api -X POST repos/azrtydxb/momo/pages -f source[branch]=gh-pages -f source[path]=/` created it.
- Step 5 left open: `gh api repos/azrtydxb/momo/pages` prints gh-pages and https://azrtydxb.github.io/momo/, but `curl -fsI https://azrtydxb.github.io/momo/llms.txt` returns 404 and will keep doing so: mike publishes only under version directories (gh-pages holds .nojekyll, dev/, versions.json), so the file is at /dev/llms.txt (200). Needs decision: change the check to /dev/llms.txt (or /latest/llms.txt once a release is deployed), or add a root redirect. The /latest/ part waits on story 256.
- Decision 2026-11-10 (`.procoder/ask/decisions.md`, "Sprint 7 docs decisions"): root copy. The deploy job copies llms.txt and llms-full.txt to the gh-pages root on every release deploy; implemented in 2a3addc, pinned by `TestDocsRootCopyOnRelease` (shown failing first on branch worktree-agent-acf2b-red-rootcopy, sha a69fbf7). Step 5 stays unticked until a release deploy shows `curl -fsI https://azrtydxb.github.io/momo/llms.txt` answering 200: root copy implemented in 2a3addc, observable at the next release.
- Step 5 observed (v0.6.0 release, 2026-11-10): `gh api repos/azrtydxb/momo/pages --jq '.source.branch, .html_url'` prints gh-pages and https://azrtydxb.github.io/momo/; `curl -fsI https://azrtydxb.github.io/momo/llms.txt` answers HTTP/2 200 after docs run 42900915565 (root copy committed 0ff5723) and Pages build run 42901318982.
