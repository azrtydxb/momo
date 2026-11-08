# momo-13 Task 6: llms.txt, llms-full.txt and raw Markdown twins

Status: open
Created: 2026-11-04
Epic: momo-13-docs-release
Sprint: -

## Description

Implements plan momo-13-docs-release Task 6: llms.txt, llms-full.txt and raw Markdown twins (`.procoder/plans/momo-13-docs-release.md:705`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-23. Files: `tools/docs/llms_hook.py`, `internal/docscheck/llms_test.go`.

## Acceptance criteria

- [ ] `TestLLMsTxt` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-23] `TestDocsCodeBlocks` extracts every Go code block from the docs and compiles it against the current module (runnable ones are executed against both databases); `TestLLMsTxt` builds the site and asserts `/llms.txt` lists every nav page, `/llms-full.txt` contains every page's Markdown, and each page has a raw `.md` twin; the docs workflow deploys `/dev/` on push to main and `/vX.Y/` plus `/latest/`…
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

- tools/docs/llms_hook.py writes llms.txt, llms-full.txt and a raw .md twin per nav page; TestLLMsTxt (build tag docs) passes in the docs workflow, run 42456103295, against the strict mkdocs build.
- Left open: TestLLMsTxt cannot be shown to have failed before the implementation (it needs a built site and the docs workflow did not exist), and the S-23 criterion also names the deployment of /dev/ and /vX.Y/ plus /latest/, which can only be observed after docs.yml reaches main.
