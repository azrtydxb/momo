# Move JSON operators to momo/jsonop

Status: done
Created: 2026-11-08
Epic: momo-14-structure-and-docs
Sprint: 006-sprint-6-split-the-root-package-into-subpackages-and-keep

## Description

Move `expr_json.go` out of the root package into `github.com/azrtydxb/momo/jsonop`. Identifiers moved: `JSONTextExpr`, `JSONText`, `JSONPath`, `JSONContains`, `JSONHasKey`. The sensitive-column propagation that `JSONTextExpr.Flags` provides is read by the core through the exported `momo.Flagged` interface from the extension API; a test proves a sensitive JSON column stays masked through extraction after the move. New names: `jsonop.Text`, `jsonop.Path`, `jsonop.Contains`, `jsonop.HasKey`. The long comment on key inlining, NULL versus JSON null and dialect behaviour moves with the code and becomes the package doc. Breaking for users: every call site and import changes and there is no alias bridge, because `jsonop` imports `momo`. Detail and the resolution of blocking methods are in section 3 of `.procoder/research/package-split.md`. Depends on stories export-expression-extension-api and move-arrayop-package.

## Acceptance criteria

- [x] the grouped-extraction golden (`expr_json_group_test.go`) and the masking test for a sensitive JSON column pass from the new package
- [x] every file listed in the description is deleted from the root package and its tests move with it; a search for each moved identifier in non-test root files, `examples/`, `internal/` and `docs/` finds no old reference (CI step output)
- [x] the new package imports only exported API of `momo` and no unexported name; the architecture test from story architecture-test-internal-and-subpackages passes with the package in its list
- [x] `internal/repocheck/testdata/moves/<package>.txt` lists every old and new identifier pair, and `docs/upgrading.md` has a row for each (the moves check from story upgrade-guide-for-the-breaking-imports passes)
- [x] every user-facing doc line and code block that named a moved identifier is updated in `README.md`, `docs/*.md`, `llms.txt` and `examples/`; the docs code-block test (momo-13 story 255) is green or, until it lands, each changed block is compiled by an example test in the new package
- [x] the moved behaviour is covered by the same unit, golden and integration tests as before, re-pointed at the new package, with no test deleted; the PostgreSQL integration job and the compile-fail job are green on CI
- [x] the commit type is `feat!` with a `BREAKING CHANGE:` footer naming the old and new import path, so release-please puts it in the 0.4.0 notes

## Evidence

- Red: run 42585252226 on the throwaway branch worktree-agent-jsonop-red (commit 4464546, now deleted). Only `jsonop/compilefail_test.go` and its programs; `TestCompileFailJSONOperators` failed with "moved compiled; want a compile error" while the old root identifiers still existed.
- Green: ci-agent run 42588329237 on commit 9cf749c (feat(jsonop)!), all jobs success: lint, apidiff-docs, unit, integration (postgres), integration-mysql (mysql84, mysql97, mariadb), fuzz-smoke, bench-module. (A later commit only adds this evidence; its own run is reported separately.)
- Criterion 1: `jsonop/jsonop_group_test.go` (TestJSONGroupByRendering golden) and `TestJSONOperatorsMaskSensitiveColumn` plus `TestJSONFlags` in `jsonop/jsonop_test.go` pass in the unit and integration jobs of run 42588329237. Masking order kept: `RenderSensitive` on the column, then `ArgFor` with those flags.
- Criterion 2: `expr_json.go` deleted; the three test files moved (renamed) to `jsonop/` and kept every test. A grep for `JSONText|JSONPath|JSONContains|JSONHasKey|JSONTextExpr` over `*.go`, `*.md`, `*.txt` finds them only in `docs/upgrading.md`, `internal/repocheck/testdata/moves/jsonop.txt`, the compile-fail program, test function names, and `.procoder/` history.
- Criterion 3: `jsonop` imports only exported momo API (Ops, NewOps, PredicateFunc, FlagsOf, Builder.RenderSensitive/ArgFor/Unsupported/SetErr/Dialect/WriteString); `feature jsonop` is in `internal/repocheck/testdata/packages.txt`; `TestKnownPackages` and the import-graph test pass in the unit job.
- Criterion 4: `internal/repocheck/testdata/moves/jsonop.txt` has five pairs; `docs/upgrading.md` has five rows; the upgrading check passes in the unit job.
- Criterion 5: docs/ai-guide.md, docs/capabilities.md, docs/TYPE_MAPPING.md and CONTRIBUTING.md updated; apidiff-docs success. `jsonop/example_test.go` adds runnable examples with checked output for Text, Path, Contains (including ErrUnsupported on SQLite) and HasKey. No `examples/` or `llms.txt` line named the identifiers.
- Criterion 6: the SQLite-live tests of both drivers (`internal/sqlshared/expr_integration_test.go`) are re-pointed at jsonop and stay green; `jsonop/jsonop_integration_test.go` moved with the code. Postgres and MySQL-family integration jobs green. Compile-fail test `TestCompileFailJSONOperators` passes in `unit`. No test deleted. `Contains` stays ErrUnsupported on SQLite, reason in the package doc.
- Criterion 7: commit 9cf749c is `feat(jsonop)!:` with a `BREAKING CHANGE:` footer naming the old identifiers, the new identifiers and the import path.
- Core exports added: none. `internal/exprtest.Pair` (a render-only two-column fixture, `internal/exprtest/pair.go`) was added for the grouped-extraction golden.
- Note: `TextExpr` (was `JSONTextExpr`) embeds `momo.Ops[string]`; `Contains` and `HasKey` are `momo.PredicateFunc` predicates.
