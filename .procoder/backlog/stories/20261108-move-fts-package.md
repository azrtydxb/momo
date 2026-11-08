# Move full-text search to momo/fts

Status: open
Created: 2026-11-08
Epic: momo-14-structure-and-docs
Sprint: -

## Description

Move `expr_fts.go` out of the root package into `github.com/azrtydxb/momo/fts`. Identifiers moved: `TSExpr`, `ToTSVector`, `WebsearchToTSQuery`, `PlainToTSQuery`, `PhraseToTSQuery`, `ToTSQuery`, `TSMatch`, `TSRank`, `TSRankCD`, `TSHeadline`. `TSExpr` keeps its methods `As` and `AppendSQL` because the type moves with them, and embeds `momo.Ops` for comparisons. New names drop the prefix (`fts.ToVector`, `fts.Rank`, `fts.Match`, `fts.Headline`). The argument types `types.TSVector` and `types.TSQuery` stay in `types`. `docs/fulltext.md` is rewritten to the new names. Breaking for users: every call site and import changes and there is no alias bridge, because `fts` imports `momo`. Detail and the resolution of blocking methods are in section 3 of `.procoder/research/package-split.md`. Depends on stories export-expression-extension-api and move-arrayop-package.

## Acceptance criteria

- [ ] the full-text integration tests (`expr_fts_integration_test.go`) run against PostgreSQL from the new package and assert the same ranks and headlines as before
- [ ] every file listed in the description is deleted from the root package and its tests move with it; a search for each moved identifier in non-test root files, `examples/`, `internal/` and `docs/` finds no old reference (CI step output)
- [ ] the new package imports only exported API of `momo` and no unexported name; the architecture test from story architecture-test-internal-and-subpackages passes with the package in its list
- [ ] `internal/repocheck/testdata/moves/<package>.txt` lists every old and new identifier pair, and `docs/upgrading.md` has a row for each (the moves check from story upgrade-guide-for-the-breaking-imports passes)
- [ ] every user-facing doc line and code block that named a moved identifier is updated in `README.md`, `docs/*.md`, `llms.txt` and `examples/`; the docs code-block test (momo-13 story 255) is green or, until it lands, each changed block is compiled by an example test in the new package
- [ ] the moved behaviour is covered by the same unit, golden and integration tests as before, re-pointed at the new package, with no test deleted; the PostgreSQL integration job and the compile-fail job are green on CI
- [ ] the commit type is `feat!` with a `BREAKING CHANGE:` footer naming the old and new import path, so release-please puts it in the 0.4.0 notes

## Evidence
