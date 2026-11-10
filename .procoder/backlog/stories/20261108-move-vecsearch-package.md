# Move pgvector search settings to momo/vecsearch

Status: done
Created: 2026-11-08
Epic: momo-14-structure-and-docs
Sprint: 006-sprint-6-split-the-root-package-into-subpackages-and-keep

## Description

Move `search.go`, except `Tx.SetLocal` out of the root package into `github.com/azrtydxb/momo/vecsearch`. Identifiers moved: `SearchSettings`, `IterativeScan`, `SearchSettings.Validate`, and the methods `Tx.SetSearch` and `DB.WithSearch` as free functions. `Tx.SetSearch` becomes `vecsearch.Apply` taking a `*momo.Tx`, `DB.WithSearch` becomes `vecsearch.With` taking a `*momo.DB` and a callback. `Tx.SetLocal` stays in core because row-level-security setup (`txsetup.go`) uses it. A search recorded in the story output confirms that `vector_expr.go` and `fields.go` do not use `SearchSettings.Validate`. The pgvector 0.8 version check that returns `ErrUnsupported` on an older server keeps working and keeps its test. Breaking for users: every call site and import changes and there is no alias bridge, because `vecsearch` imports `momo`. Detail and the resolution of blocking methods are in section 3 of `.procoder/research/package-split.md`. Depends on stories export-expression-extension-api and move-arrayop-package.

## Acceptance criteria

- [x] the iterative-scan and `hnsw.max_scan_tuples` integration tests pass through the new package, including the `ErrUnsupported` result on an older pgvector
- [x] every file listed in the description is deleted from the root package and its tests move with it; a search for each moved identifier in non-test root files, `examples/`, `internal/` and `docs/` finds no old reference (CI step output)
- [x] the new package imports only exported API of `momo` and no unexported name; the architecture test from story architecture-test-internal-and-subpackages passes with the package in its list
- [x] `internal/repocheck/testdata/moves/<package>.txt` lists every old and new identifier pair, and `docs/upgrading.md` has a row for each (the moves check from story upgrade-guide-for-the-breaking-imports passes)
- [x] every user-facing doc line and code block that named a moved identifier is updated in `README.md`, `docs/*.md`, `llms.txt` and `examples/`; the docs code-block test (momo-13 story 255) is green or, until it lands, each changed block is compiled by an example test in the new package
- [x] the moved behaviour is covered by the same unit, golden and integration tests as before, re-pointed at the new package, with no test deleted; the PostgreSQL integration job and the compile-fail job are green on CI
- [x] the commit type is `feat!` with a `BREAKING CHANGE:` footer naming the old and new import path, so release-please puts it in the 0.4.0 notes

## Evidence

- Red first: ci-agent run 42601026558 on the throwaway branch worktree-agent-afa103767353b7284-red (headSha eab8b68, since deleted). It carried only vecsearch/compilefail_test.go, its fixture and a doc-only vecsearch/doc.go (the commit gate refuses a test-only package), with the old root code intact. Unit, PostgreSQL integration and the MySQL integration jobs failed with `TestCompileFailSearchSettings: moved compiled; want a compile error`.
- Green: ci-agent run 42603818502, headSha 9c8293a, all nine jobs success: lint, unit, apidiff-docs, fuzz-smoke, bench-module, integration (postgres), integration-mysql mysql84, mysql97 and mariadb. Run 42601001605 on an earlier commit failed on a missing vecsearch import in bench/vector/topk_integration_test.go and in the README quickstart snippet (the README snippet test requires the snippet verbatim in examples/quickstart); both fixed.
- Core search: `Validate` is used only by search.go; vector_expr.go and fields.go do not use SearchSettings (grep over root non-test files). The only other root reference was a comment in txsetup.go, now `vecsearch.With`. No core export was added: `Tx.Dialect`, `Scalars`, `Error`, `ErrUnsupported`, `ErrValidation`, `DB.Tx` and `Tx.SetLocal` already exist. `Tx.SetLocal` moved from search.go to txsetup.go with root tests `TestSetLocalBindsNameAndValue` and `TestSetLocalUnsupportedOffPostgres`.
- Criterion 1: TestFilteredANNIterativeScan, TestSearchIterativeSettingsOnInstalledPgvector, TestSearchScanMemMultiplierScoped, TestExactSearchBypassesVectorIndex and TestSearchSettingsScoped moved to vecsearch/*_integration_test.go and pass in the PostgreSQL integration job; TestSearchIterativeNeedsPgvector08 and TestSearchScanMemMultiplierNeedsPgvector08 (fake driver, version 0.7.4 gives ErrUnsupported with no set_config) pass in unit. hnsw.max_scan_tuples is covered by TestFilteredANNIterativeScan and TestSearchIterativeSettingsOnInstalledPgvector.
- Criterion 2: search.go and its four test files are deleted from the root (moved into vecsearch/). A grep for SearchSettings, SetSearch, WithSearch and the Iterative* constants over root, examples/, internal/, docs/ and bench/ finds only docs/upgrading.md rows, the moves file and the compile-fail fixture.
- Criterion 3: vecsearch imports only exported momo API (Tx.Dialect, Scalars, Error, DB.Tx, Tx.SetLocal); packages.txt lists `feature vecsearch`; the architecture tests pass in the unit job.
- Criterion 4: internal/repocheck/testdata/moves/vecsearch.txt has eight pairs and docs/upgrading.md eight rows; TestUpgradingCoversMoves passes in unit. SearchSettings becomes vecsearch.Settings, Tx.SetSearch becomes Apply, DB.WithSearch becomes With, the rest keep their names.
- Criterion 5: README.md, docs/vectors.md, docs/ai-guide.md, docs/capabilities.md, docs/supported-versions.md, examples/quickstart, examples/vector-search and the example tests (ExampleWith, ExampleApply in vecsearch/example_test.go) are updated; the README snippet test and docs code-block tests pass.
- Criterion 6: the unit, fake-driver and integration tests moved with the package re-pointed at vecsearch; two tests were added (TestWithAppliesSettingsAndRunsCallback, TestWithRejectsInvalidSettingsBeforeBegin); none deleted. The SQLite search settings check stays in vector_sqlite_integration_test.go and calls vecsearch.With. Unit, PostgreSQL integration and the compile-fail tests are green.
- Criterion 7: commit d3fa8ff is `feat(vecsearch)!:` with a BREAKING CHANGE footer naming the old identifiers, the new ones and the import path.
