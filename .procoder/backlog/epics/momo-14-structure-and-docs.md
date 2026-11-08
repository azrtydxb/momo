# momo-14-structure-and-docs

Status: open
Created: 2026-11-08

## Description

Delivers the 2026-11-08 decision "Root-package structure and always-current docs" (`.procoder/ask/decisions.md`): after sprint-5 lands on main, a dedicated sprint moves feature APIs out of the root `momo` package into subpackages (a breaking change before v1), moves non-API mechanics into `internal/`, enforces a file-prefix taxonomy and an architecture test, and adds the docs-freshness guards (one source for version strings, generated capability and type tables with a drift check, a docs-reference-exists test, an apidiff docs gate, a stale-docs sweep, an upgrade guide). The plan, inventory and ordering are in `.procoder/research/package-split.md`.

It has no milestone: it is cross-cutting hygiene before 1.0. It overlaps the docs-site epic `momo-13-docs-release` in one place only: the compile-and-run check of every Go code block in the docs is momo-13 story 255 (`TestDocsCodeBlocks`), and momo-14 does not build a second one. Stories here that rewrite import paths in docs depend on 255 or hand-check their blocks until it lands. llms.txt, llms-full.txt and the agent guide are momo-13 stories 256 and 257; momo-14 only adds `docs/upgrading.md` to the pages they index.

Ordering (see section 6 of the research note): guards first (they touch no root file), then the additive expression-extension API, then the pilot move (`arrayop`), then the remaining leaf moves in parallel, then generated docs and the reference test, then the file-prefix rename, then the 0.4.0 release.
