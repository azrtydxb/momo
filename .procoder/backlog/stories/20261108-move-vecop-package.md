# Move vector distance builders to momo/vecop

Status: done
Created: 2026-11-08
Epic: momo-14-structure-and-docs
Sprint: 006-sprint-6-split-the-root-package-into-subpackages-and-keep

## Description

Move `vector_expr.go` out of the root package into `github.com/azrtydxb/momo/vecop`. Identifiers moved: `DistanceExpr`, `L2Distance`, `InnerProductDistance`, `CosineDistance`, `L1Distance`, `HammingDistance`, `JaccardDistance`. Optional and last: `VectorSource[V]` has the unexported method `vectorColumn`, so the extension-API story must have exported its accessor. The vector column types themselves (`VectorColumn` and its four variants) stay in core because generated code names them and `vector` is imported by the core, so they cannot move into `momo/vector`. New names: `vecop.L2`, `vecop.Cosine`, `vecop.InnerProduct`, `vecop.L1`, `vecop.Hamming`, `vecop.Jaccard`. If the accessor turns out to leak more than it is worth, close this story as dropped with the reason recorded. Breaking for users: every call site and import changes and there is no alias bridge, because `vecop` imports `momo`. Detail and the resolution of blocking methods are in section 3 of `.procoder/research/package-split.md`. Depends on stories export-expression-extension-api and move-arrayop-package.

## Acceptance criteria

- [x] the distance builders render the same SQL for pgvector operators and sqlite-vec functions (golden tests re-pointed), and `DistanceExpr.Desc` still orders descending
- [x] every file listed in the description is deleted from the root package and its tests move with it; a search for each moved identifier in non-test root files, `examples/`, `internal/` and `docs/` finds no old reference (CI step output)
- [x] the new package imports only exported API of `momo` and no unexported name; the architecture test from story architecture-test-internal-and-subpackages passes with the package in its list
- [x] `internal/repocheck/testdata/moves/<package>.txt` lists every old and new identifier pair, and `docs/upgrading.md` has a row for each (the moves check from story upgrade-guide-for-the-breaking-imports passes)
- [x] every user-facing doc line and code block that named a moved identifier is updated in `README.md`, `docs/*.md`, `llms.txt` and `examples/`; the docs code-block test (momo-13 story 255) is green or, until it lands, each changed block is compiled by an example test in the new package
- [x] the moved behaviour is covered by the same unit, golden and integration tests as before, re-pointed at the new package, with no test deleted; the PostgreSQL integration job and the compile-fail job are green on CI
- [x] the commit type is `feat!` with a `BREAKING CHANGE:` footer naming the old and new import path, so release-please puts it in the 0.4.0 notes

## Evidence

- Red: ci-agent run 42601065016 on 50a31c2 (throwaway branch, deleted): TestCompileFailVectorDistances fails with "moved compiled; want a compile error".
- Green: ci-agent run 42604075347, headSha 1d7bc82, all jobs success (lint, unit, integration postgres, mysql84, mysql97, mariadb, apidiff-docs, bench-module, fuzz-smoke). An earlier run, 42601609064, failed on the README import block and an unused root map; both fixed in 1d7bc82.
- Criteria 1 and 6: vecop unit, masking, example and compile-fail tests plus the re-pointed PostgreSQL, SQLite and MariaDB integration tests pass in that run; Desc is asserted in vecop TestDistanceInQueries.
- Criterion 2: vector_expr.go and vector_expr_test.go are deleted; the old names remain only as test labels and in internal/vectorparity, which has its own unrelated helpers.
- Criterion 3: vecop imports only exported momo API (Ops, VectorColumnOf, Builder methods); TestFeaturePackagesImportOnlyAllowed passes with `feature vecop`. No core export was added.
- Criterion 4: internal/repocheck/testdata/moves/vecop.txt and seven docs/upgrading.md rows; the upgrade check passes.
- Criterion 5: README, docs/vectors.md, ai-guide.md, mysql.md, fulltext.md, capabilities.md, llms.txt and both examples updated; the README snippet test passes.
- Criterion 7: commit 524eaa6 is feat(vecop)! with a BREAKING CHANGE footer.
