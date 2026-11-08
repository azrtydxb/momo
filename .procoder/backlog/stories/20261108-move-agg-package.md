# Move advanced aggregates and JSON builders to momo/agg

Status: open
Created: 2026-11-08
Epic: momo-14-structure-and-docs
Sprint: -

## Description

Move `expr_agg.go` out of the root package into `github.com/azrtydxb/momo/agg`. Identifiers moved: `AggExpr`, `Aggregate`, `StringAgg`, `ArrayAgg`, `JSONBAgg`, `PercentileCont`, `PercentileDisc`, `JSONPair`, `JSONField`, `JSONObjectExpr`, `JSONBBuildObject`, `ToJSONExpr`, `ToJSONB`, `EmptyJSONArray`. `AggExpr.OrderBy`, `WithinGroup`, `Filter`, `As` and `AppendSQL` move with the type. The basic `Agg[T]`, `CountAll`, `CountOf`, `CountDistinct`, `Sum`, `Avg`, `Min`, `Max` stay in core because `Having` and select lists are typed on `Agg`. New names: `agg.String`, `agg.Array`, `agg.JSONB`, `agg.PercentileCont`, `agg.BuildObject`. Breaking for users: every call site and import changes and there is no alias bridge, because `agg` imports `momo`. Detail and the resolution of blocking methods are in section 3 of `.procoder/research/package-split.md`. Depends on stories export-expression-extension-api and move-arrayop-package.

## Acceptance criteria

- [ ] an example test shows `agg.String` with ORDER BY inside a `Having` clause built from core `momo.Agg`, proving the two packages compose
- [ ] every file listed in the description is deleted from the root package and its tests move with it; a search for each moved identifier in non-test root files, `examples/`, `internal/` and `docs/` finds no old reference (CI step output)
- [ ] the new package imports only exported API of `momo` and no unexported name; the architecture test from story architecture-test-internal-and-subpackages passes with the package in its list
- [ ] `internal/repocheck/testdata/moves/<package>.txt` lists every old and new identifier pair, and `docs/upgrading.md` has a row for each (the moves check from story upgrade-guide-for-the-breaking-imports passes)
- [ ] every user-facing doc line and code block that named a moved identifier is updated in `README.md`, `docs/*.md`, `llms.txt` and `examples/`; the docs code-block test (momo-13 story 255) is green or, until it lands, each changed block is compiled by an example test in the new package
- [ ] the moved behaviour is covered by the same unit, golden and integration tests as before, re-pointed at the new package, with no test deleted; the PostgreSQL integration job and the compile-fail job are green on CI
- [ ] the commit type is `feat!` with a `BREAKING CHANGE:` footer naming the old and new import path, so release-please puts it in the 0.4.0 notes

## Evidence
