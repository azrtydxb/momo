# Move scalar SQL functions to momo/sqlfn

Status: done
Created: 2026-11-08
Epic: momo-14-structure-and-docs
Sprint: 006-sprint-6-split-the-root-package-into-subpackages-and-keep

## Description

Move `expr_funcs.go`, except `Func` and `FuncExpr` out of the root package into `github.com/azrtydxb/momo/sqlfn`. Identifiers moved: `Substr`, `SubstrFrom`, `Lower`, `Upper`, `Length`, `LengthExpr`, `CurrentSchema`, `CurrentSchemaExpr`, `Case`, `CaseExpr`, `Numeric`, `ArithExpr`, `Add`, `AddExpr`, `Sub`, `SubExpr`, `Mul`, `MulExpr`, `CoalesceExpr`, `Cast`, `CastExpr`, `Concat`, `ConcatExpr`. `Func` and `FuncExpr` stay in core as the generic escape hatch and `sqlfn` builds on `momo.Func`. `Coalesce`, `Value`, `SQL` and `TypedSQL` stay in `expr.go`. `CaseExpr.When`, `WhenExpr`, `Else`, `ElseExpr` and the other methods move with their types. The `Numeric` constraint moves too; core uses `Number` only. Breaking for users: every call site and import changes and there is no alias bridge, because `sqlfn` imports `momo`. Detail and the resolution of blocking methods are in section 3 of `.procoder/research/package-split.md`. Depends on stories export-expression-extension-api and move-arrayop-package.

## Acceptance criteria

- [x] the `CaseExpr` zero-value test (a `CaseExpr` built without its constructor fails the build) is kept and passes from the new package
- [x] every file listed in the description is deleted from the root package and its tests move with it; a search for each moved identifier in non-test root files, `examples/`, `internal/` and `docs/` finds no old reference (CI step output)
- [x] the new package imports only exported API of `momo` and no unexported name; the architecture test from story architecture-test-internal-and-subpackages passes with the package in its list
- [x] `internal/repocheck/testdata/moves/<package>.txt` lists every old and new identifier pair, and `docs/upgrading.md` has a row for each (the moves check from story upgrade-guide-for-the-breaking-imports passes)
- [x] every user-facing doc line and code block that named a moved identifier is updated in `README.md`, `docs/*.md`, `llms.txt` and `examples/`; the docs code-block test (momo-13 story 255) is green or, until it lands, each changed block is compiled by an example test in the new package
- [x] the moved behaviour is covered by the same unit, golden and integration tests as before, re-pointed at the new package, with no test deleted; the PostgreSQL integration job and the compile-fail job are green on CI
- [x] the commit type is `feat!` with a `BREAKING CHANGE:` footer naming the old and new import path, so release-please puts it in the 0.4.0 notes

## Evidence

- Red: ci-agent run 42586267800 (dispatched on throwaway branch sqlfn-red at 449bf2d, since deleted): TestCompileFailSQLFunctions failed because the old momo.* names still compiled; unit, integration and apidiff-docs jobs failed.
- Green: ci-agent run 42588877389, headSha 0ed67eaa8b08a6a194bcfeb3433e8764d1d3b1d9, all jobs success (unit, lint, fuzz-smoke, integration postgres, integration-mysql mysql84/mysql97/mariadb, apidiff-docs, bench-module). Earlier run 42586310840 (d15c348) failed on sqlfn test fixtures only and was fixed in 0ed67ea.
- Zero-value CaseExpr: sqlfn/sqlfn_test.go TestZeroCaseComparisonFailsBuild passes in the unit job.
- No old reference: the unit job runs the repocheck moves, upgrading and docs tests; grep of root non-test files, examples/, internal/ and docs/ for momo.<moved name> found none outside docs/upgrading.md.
- Imports: sqlfn imports only momo and dialect (TestFeaturePackagesImportOnlyAllowed, `feature sqlfn` in packages.txt) and uses only exported API. One core export was added: momo.ValidTypeName, tested by TestValidTypeName in expr_funcs_test.go.
- Moves: internal/repocheck/testdata/moves/sqlfn.txt and 23 rows in docs/upgrading.md; docs/ai-guide.md, docs/fulltext.md, docs/capabilities.md updated; docs code blocks are parse-only fragments.
- Tests: unit goldens, masking, examples, compile-fail (make test-compilefail pattern TestCompileFail*) and integration tests moved to sqlfn/; integration (postgres, mysql, mariadb) jobs green.
- Commit type: feat(sqlfn)! with BREAKING CHANGE footer (commit 30e119f).
