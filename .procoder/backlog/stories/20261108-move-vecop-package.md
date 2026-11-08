# Move vector distance builders to momo/vecop

Status: open
Created: 2026-11-08
Epic: momo-14-structure-and-docs
Sprint: 006-sprint-6-split-the-root-package-into-subpackages-and-keep

## Description

Move `vector_expr.go` out of the root package into `github.com/azrtydxb/momo/vecop`. Identifiers moved: `DistanceExpr`, `L2Distance`, `InnerProductDistance`, `CosineDistance`, `L1Distance`, `HammingDistance`, `JaccardDistance`. Optional and last: `VectorSource[V]` has the unexported method `vectorColumn`, so the extension-API story must have exported its accessor. The vector column types themselves (`VectorColumn` and its four variants) stay in core because generated code names them and `vector` is imported by the core, so they cannot move into `momo/vector`. New names: `vecop.L2`, `vecop.Cosine`, `vecop.InnerProduct`, `vecop.L1`, `vecop.Hamming`, `vecop.Jaccard`. If the accessor turns out to leak more than it is worth, close this story as dropped with the reason recorded. Breaking for users: every call site and import changes and there is no alias bridge, because `vecop` imports `momo`. Detail and the resolution of blocking methods are in section 3 of `.procoder/research/package-split.md`. Depends on stories export-expression-extension-api and move-arrayop-package.

## Acceptance criteria

- [ ] the distance builders render the same SQL for pgvector operators and sqlite-vec functions (golden tests re-pointed), and `DistanceExpr.Desc` still orders descending
- [ ] every file listed in the description is deleted from the root package and its tests move with it; a search for each moved identifier in non-test root files, `examples/`, `internal/` and `docs/` finds no old reference (CI step output)
- [ ] the new package imports only exported API of `momo` and no unexported name; the architecture test from story architecture-test-internal-and-subpackages passes with the package in its list
- [ ] `internal/repocheck/testdata/moves/<package>.txt` lists every old and new identifier pair, and `docs/upgrading.md` has a row for each (the moves check from story upgrade-guide-for-the-breaking-imports passes)
- [ ] every user-facing doc line and code block that named a moved identifier is updated in `README.md`, `docs/*.md`, `llms.txt` and `examples/`; the docs code-block test (momo-13 story 255) is green or, until it lands, each changed block is compiled by an example test in the new package
- [ ] the moved behaviour is covered by the same unit, golden and integration tests as before, re-pointed at the new package, with no test deleted; the PostgreSQL integration job and the compile-fail job are green on CI
- [ ] the commit type is `feat!` with a `BREAKING CHANGE:` footer naming the old and new import path, so release-please puts it in the 0.4.0 notes

## Evidence
