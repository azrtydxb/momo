# Move array predicates to momo/arrayop (pilot move)

Status: open
Created: 2026-11-08
Epic: momo-14-structure-and-docs
Sprint: 006-sprint-6-split-the-root-package-into-subpackages-and-keep

## Description

First of the leaf moves and the pilot, because `array_ops.go` is the smallest (75 lines) and proves the pattern, the `moves/` file, the docs edit and the compile-fail test before the larger moves. New import path `github.com/azrtydxb/momo/arrayop`; identifiers moved: `ArrayContains`, `ArrayContainedBy`, `ArrayOverlaps`, `AnyOf`, with the unexported `pgArrayPred` and `boundSlice`. Names in the new package drop the prefix: `arrayop.Contains`, `arrayop.ContainedBy`, `arrayop.Overlaps`, `arrayop.AnyOf`. The predicate type embeds `momo.Ops[bool]` and uses `ArgFor`, `RenderSensitive`, `Unsupported` and `FlagsOf` from the extension API. Breaking for users: every call site and import changes; no alias is possible because `arrayop` imports `momo`. Depends on story export-expression-extension-api.

## Acceptance criteria

- [ ] every file listed in the description is deleted from the root package and its tests move with it; a search for each moved identifier in non-test root files, `examples/`, `internal/` and `docs/` finds no old reference (CI step output)
- [ ] the new package imports only exported API of `momo` and no unexported name; the architecture test from story architecture-test-internal-and-subpackages passes with the package in its list
- [ ] `internal/repocheck/testdata/moves/<package>.txt` lists every old and new identifier pair, and `docs/upgrading.md` has a row for each (the moves check from story upgrade-guide-for-the-breaking-imports passes)
- [ ] every user-facing doc line and code block that named a moved identifier is updated in `README.md`, `docs/*.md`, `llms.txt` and `examples/`; the docs code-block test (momo-13 story 255) is green or, until it lands, each changed block is compiled by an example test in the new package
- [ ] the moved behaviour is covered by the same unit, golden and integration tests as before, re-pointed at the new package, with no test deleted; the PostgreSQL integration job and the compile-fail job are green on CI
- [ ] the commit type is `feat!` with a `BREAKING CHANGE:` footer naming the old and new import path, so release-please puts it in the 0.4.0 notes

## Evidence
