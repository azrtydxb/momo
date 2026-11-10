# momo-13 Task 6: llms.txt, llms-full.txt and raw Markdown twins

Status: open
Created: 2026-11-04
Epic: momo-13-docs-release
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

Implements plan momo-13-docs-release Task 6: llms.txt, llms-full.txt and raw Markdown twins (`.procoder/plans/momo-13-docs-release.md:705`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-23. Files: `tools/docs/llms_hook.py`, `internal/docscheck/llms_test.go`.

## Acceptance criteria

- [x] `TestLLMsTxt` exists (`internal/docscheck/llms_test.go:33`), fails when a raw twin is broken (mutation run below), and passes now
- [ ] [S-23] `TestDocsCodeBlocks` extracts every Go code block from the docs and compiles it against the current module (runnable ones are executed against both databases); `TestLLMsTxt` builds the site and asserts `/llms.txt` lists every nav page, `/llms-full.txt` contains every page's Markdown, and each page has a raw `.md` twin; the docs workflow deploys `/dev/` on push to main and `/vX.Y/` plus `/latest/`…
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- tools/docs/llms_hook.py writes llms.txt, llms-full.txt and a raw .md twin per nav page; TestLLMsTxt (build tag docs) passes in the docs workflow, run 42456103295, against the strict mkdocs build.
- Left open: TestLLMsTxt cannot be shown to have failed before the implementation (it needs a built site and the docs workflow did not exist), and the S-23 criterion also names the deployment of /dev/ and /vX.Y/ plus /latest/, which can only be observed after docs.yml reaches main.
- Mutation evidence (the test cannot be shown red before its implementation, so it is shown red on a deliberate break): branch worktree-agent-ae0b59d3910467120-red, sha 89d1113, with `InsertReturning` renamed in docs/capabilities.md and the raw twin writer in tools/docs/llms_hook.py emptied: unit job and integration (postgres) job of ci-agent run 42768946200 FAIL TestCapabilityMatrixComplete (`docs/capabilities.md does not mention the dialect capability InsertReturning`); docs run 42768946297 FAIL TestLLMsTxt.
- Green: ci-agent run 42768951858 (sha 3e1eed073aed17297798962fc042066a62631848, success) and docs run 42768951678 (same sha, success). The commits carrying this work were made through the procoder commit gate.
- S-23 deploy part: /dev/ is published (docs run 42767105279 on main, deploy job success; https://azrtydxb.github.io/momo/dev/llms.txt returns 200). /vX.Y/ and /latest/ were never published: no docs run exists for tag v0.4.0, because the tag release-please creates with the workflow token does not trigger a tag push. Fixed in 3e1eed0: release-please.yml dispatches docs.yml with the tag, docs.yml deploys a dispatched release tag (pinned by `TestDocsPublishedOnRelease`, internal/repocheck/docs_workflow_test.go). The criterion stays unticked until the next release (or `gh workflow run docs.yml --ref main -f tag=v0.4.0` after this reaches main) shows /v0.4/ and /latest/ on gh-pages.
