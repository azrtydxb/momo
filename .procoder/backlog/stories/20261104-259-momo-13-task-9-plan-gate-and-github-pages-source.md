# momo-13 Task 9: Plan gate and GitHub Pages source

Status: open
Created: 2026-11-04
Epic: momo-13-docs-release
Sprint: -

## Description

Implements plan momo-13-docs-release Task 9: Plan gate and GitHub Pages source (`.procoder/plans/momo-13-docs-release.md:985`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-21, S-23.

## Acceptance criteria

- [ ] [S-21] `TestExamples` runs every example end to end against both databases in CI; docs/ contains architecture, capability matrix, migration safety and recovery guide, contributor guide and supported versions; fails if an example is not wired into the test.
- [ ] [S-23] `TestDocsCodeBlocks` extracts every Go code block from the docs and compiles it against the current module (runnable ones are executed against both databases); `TestLLMsTxt` builds the site and asserts `/llms.txt` lists every nav page, `/llms-full.txt` contains every page's Markdown, and each page has a raw `.md` twin; the docs workflow deploys `/dev/` on push to main and `/vX.Y/` plus `/latest/`…
- [ ] Step 1: Run the Go gate. `make fmt-check vet test` and `go test ./internal/docscheck/ ./internal/repocheck/ -count=1`. Expected: exit 0 for each.
- [ ] Step 2: Run the example and block gate against the PostgreSQL service. `make db-up`, then `make test-integration-postgres` (the examples and the runnable blocks run through `testopen.Each`). Expected: `ok`. Then `make db-down`.
- [ ] Step 3: Build the site as CI does. With the venv from Task 6: `mkdocs build --strict --site-dir "$SCRATCH/site"` exits 0, then `MOMO_DOCS_SITE="$SCRATCH/site" go test -tags docs -count=1 -run '^TestLLMsTxt$' ./internal/docscheck/` prints `ok`.
- [ ] Step 4: Set the Pages source after the first deploy. The `gh-pages` branch is created by the first `mike deploy --push dev` on main (Task 8), so run this step after that deploy succeeds. Requires repository administration rights.
- [ ] Step 5: Verify the Pages source and the published files. `gh api repos/azrtydxb/momo/pages --jq '.source.branch, .html_url'` must print `gh-pages` and `https://azrtydxb.github.io/momo/`. Then `curl -fsI https://azrtydxb.github.io/momo/llms.txt` must report `HTTP/2 200` (retry for up to ten minutes…
- [ ] `procoder check` gate is clean

## Evidence

