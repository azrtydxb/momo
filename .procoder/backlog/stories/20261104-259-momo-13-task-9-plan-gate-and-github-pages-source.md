# momo-13 Task 9: Plan gate and GitHub Pages source

Status: open
Created: 2026-11-04
Epic: momo-13-docs-release
Sprint: -

## Description

Implements plan momo-13-docs-release Task 9: Plan gate and GitHub Pages source (`.procoder/plans/momo-13-docs-release.md:985`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-21, S-23. Contributes to S-21, S-23; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] Step 1: Run the Go gate. `make fmt-check vet test` and `go test ./internal/docscheck/ ./internal/repocheck/ -count=1`. Expected: exit 0 for each.
- [ ] Step 2: Run the example and block gate against the PostgreSQL service. `make db-up`, then `make test-integration-postgres` (the examples and the runnable blocks run through `testopen.Each`). Expected: `ok`. Then `make db-down`.
- [ ] Step 3: Build the site as CI does. With the venv from Task 6: `mkdocs build --strict --site-dir "$SCRATCH/site"` exits 0, then `MOMO_DOCS_SITE="$SCRATCH/site" go test -tags docs -count=1 -run '^TestLLMsTxt$' ./internal/docscheck/` prints `ok`.
- [ ] Step 4: Set the Pages source after the first deploy. The `gh-pages` branch is created by the first `mike deploy --push dev` on main (Task 8), so run this step after that deploy succeeds. Requires repository administration rights.
- [ ] Step 5: Verify the Pages source and the published files. `gh api repos/azrtydxb/momo/pages --jq '.source.branch, .html_url'` must print `gh-pages` and `https://azrtydxb.github.io/momo/`. Then `curl -fsI https://azrtydxb.github.io/momo/llms.txt` must report `HTTP/2 200` (retry for up to ten minutes…
- [ ] `procoder check` gate is clean

## Evidence

- Steps 1 to 3 are covered by the docs workflow, run 42456103295: strict mkdocs build, TestLLMsTxt, code blocks and runnable blocks.
- Left open: steps 4 and 5 (set the Pages source on gh-pages, check the published llms.txt) need the first mike deploy from main and repository administration rights; they cannot run before docs.yml is on main.
