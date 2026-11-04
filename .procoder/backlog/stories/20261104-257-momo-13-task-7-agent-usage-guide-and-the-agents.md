# momo-13 Task 7: Agent usage guide and the AGENTS.md snippet

Status: open
Created: 2026-11-04
Epic: momo-13-docs-release
Sprint: -

## Description

Implements plan momo-13-docs-release Task 7: Agent usage guide and the AGENTS.md snippet (`.procoder/plans/momo-13-docs-release.md:818`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-23. Files: `docs/agents.md`, `internal/docscheck/agents_test.go`.

## Acceptance criteria

- [ ] `TestAgentGuideSnippet` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-23] `TestDocsCodeBlocks` extracts every Go code block from the docs and compiles it against the current module (runnable ones are executed against both databases); `TestLLMsTxt` builds the site and asserts `/llms.txt` lists every nav page, `/llms-full.txt` contains every page's Markdown, and each page has a raw `.md` twin; the docs workflow deploys `/dev/` on push to main and `/vX.Y/` plus `/latest/`…
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

