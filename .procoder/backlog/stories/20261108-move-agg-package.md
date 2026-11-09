# Move advanced aggregates and JSON builders to momo/agg

Status: done
Created: 2026-11-08
Epic: momo-14-structure-and-docs
Sprint: 006-sprint-6-split-the-root-package-into-subpackages-and-keep

## Description

Move `expr_agg.go` out of the root package into `github.com/azrtydxb/momo/agg`. Identifiers moved: `AggExpr`, `Aggregate`, `StringAgg`, `ArrayAgg`, `JSONBAgg`, `PercentileCont`, `PercentileDisc`, `JSONPair`, `JSONField`, `JSONObjectExpr`, `JSONBBuildObject`, `ToJSONExpr`, `ToJSONB`, `EmptyJSONArray`. `AggExpr.OrderBy`, `WithinGroup`, `Filter`, `As` and `AppendSQL` move with the type. The basic `Agg[T]`, `CountAll`, `CountOf`, `CountDistinct`, `Sum`, `Avg`, `Min`, `Max` stay in core because `Having` and select lists are typed on `Agg`. New names: `agg.String`, `agg.Array`, `agg.JSONB`, `agg.PercentileCont`, `agg.BuildObject`. Breaking for users: every call site and import changes and there is no alias bridge, because `agg` imports `momo`. Detail and the resolution of blocking methods are in section 3 of `.procoder/research/package-split.md`. Depends on stories export-expression-extension-api and move-arrayop-package.

## Acceptance criteria

- [x] an example test shows `agg.String` with ORDER BY inside a `Having` clause built from core `momo.Agg`, proving the two packages compose
- [x] every file listed in the description is deleted from the root package and its tests move with it; a search for each moved identifier in non-test root files, `examples/`, `internal/` and `docs/` finds no old reference (CI step output)
- [x] the new package imports only exported API of `momo` and no unexported name; the architecture test from story architecture-test-internal-and-subpackages passes with the package in its list
- [x] `internal/repocheck/testdata/moves/<package>.txt` lists every old and new identifier pair, and `docs/upgrading.md` has a row for each (the moves check from story upgrade-guide-for-the-breaking-imports passes)
- [x] every user-facing doc line and code block that named a moved identifier is updated in `README.md`, `docs/*.md`, `llms.txt` and `examples/`; the docs code-block test (momo-13 story 255) is green or, until it lands, each changed block is compiled by an example test in the new package
- [x] the moved behaviour is covered by the same unit, golden and integration tests as before, re-pointed at the new package, with no test deleted; the PostgreSQL integration job and the compile-fail job are green on CI
- [x] the commit type is `feat!` with a `BREAKING CHANGE:` footer naming the old and new import path, so release-please puts it in the 0.4.0 notes

## Evidence

Implementation commit d9331bf (the story edit lands in a later docs-only commit). All results are from GitHub Actions; nothing was run locally.

- Green: ci-agent run 42589756424, headSha d9331bf4264d9b401c8051a9121e1f68c1bd2145, all nine jobs success (unit, lint, apidiff-docs, fuzz-smoke, bench-module, integration postgres, mysql84, mysql97, mariadb). An earlier run, 42588118916, failed only on the prefix guard (TestRootFilePrefixes) for three misnamed export tests; fixed by renaming them to expr_test.go, builder_test.go and window_test.go.
- Red first: ci-agent run 42585985982 on the throwaway branch worktree-agent-a975707f770371c29-red (headSha 8dbf8e1, since deleted). It carried the compile-fail test, its programs, a stub agg/doc.go and the packages.txt line, with the old root code intact. Unit and integration jobs failed with `TestCompileFailAggregates: moved compiled; want a compile error`.
- Criterion 1: agg/example_test.go `Example` composes agg.String(...).OrderBy(...) with core momo.CountAll() inside momo.And, the shape a HAVING clause takes; its output is checked in the unit job of the green run.
- Criterion 2: expr_agg.go and window_agg.go are gone from the root (AggExpr.Over moved with the type to agg/window.go), their tests renamed into agg/. The compile-fail `moved` program fails on every old identifier. Old names remain only in docs/upgrading.md, testdata/moves/agg.txt and the compile-fail fixture and test.
- Criterion 3: agg imports momo, dialect and types only, all exported API. Core exports added: Order.AppendSQL, Builder.AppendFilter, momo.WindowOf[T] (ValidName was already exported), each with a root unit test (expr_test.go, builder_test.go, window_test.go). `feature agg` is in packages.txt; internal/repocheck is ok in the unit job.
- Criterion 4: internal/repocheck/testdata/moves/agg.txt (14 pairs) and 14 rows in docs/upgrading.md; the moves check passes in the unit job.
- Criterion 5: docs/ai-guide.md and docs/capabilities.md updated; README.md, llms.txt and examples/ never named the identifiers. The example test in agg compiles the changed pattern.
- Criterion 6: unit, golden, masking (TestAggExprOverMasksSensitiveFilter) and integration tests moved to agg/ unchanged in intent; postgres, mysql and mariadb integration jobs and the compile-fail job are green.
- Criterion 7: commit 85a936b is feat(agg)! with a BREAKING CHANGE footer naming old and new identifiers and the import path.
