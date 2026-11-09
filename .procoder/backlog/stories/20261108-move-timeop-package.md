# Move date and time expressions to momo/timeop

Status: done
Created: 2026-11-08
Epic: momo-14-structure-and-docs
Sprint: 006-sprint-6-split-the-root-package-into-subpackages-and-keep

## Description

Move `expr_time.go` out of the root package into `github.com/azrtydxb/momo/timeop`. Identifiers moved: `ExtractField` and its constants, `ExtractExpr`, `Extract`, `TimeExpr`, `AtTimeZone`, `AtTimeZoneExpr`, `IntervalUnit`, `Interval`, `IntervalOf`, `IntervalExpr`, `AddInterval`, `SubInterval`, `Now`, `CurrentTimestamp`, `TruncUnit` and its constants, `DateTrunc`. Uses `momo.Typed` and `momo.Ops` from the extension API. Before moving, a search confirms the core does not call the exported `Now` (it uses `time.Now`); the story output records the search. The golden SQL for PostgreSQL, SQLite and MySQL is kept as it is, and story sqlite-json-and-time-operators-live depends on this move. Breaking for users: every call site and import changes and there is no alias bridge, because `timeop` imports `momo`. Detail and the resolution of blocking methods are in section 3 of `.procoder/research/package-split.md`. Depends on stories export-expression-extension-api and move-arrayop-package.

## Acceptance criteria

- [x] the three-dialect golden tests for Extract, DateTrunc, interval arithmetic and Now pass unchanged from the new package
- [x] every file listed in the description is deleted from the root package and its tests move with it; a search for each moved identifier in non-test root files, `examples/`, `internal/` and `docs/` finds no old reference (CI step output)
- [x] the new package imports only exported API of `momo` and no unexported name; the architecture test from story architecture-test-internal-and-subpackages passes with the package in its list
- [x] `internal/repocheck/testdata/moves/<package>.txt` lists every old and new identifier pair, and `docs/upgrading.md` has a row for each (the moves check from story upgrade-guide-for-the-breaking-imports passes)
- [x] every user-facing doc line and code block that named a moved identifier is updated in `README.md`, `docs/*.md`, `llms.txt` and `examples/`; the docs code-block test (momo-13 story 255) is green or, until it lands, each changed block is compiled by an example test in the new package
- [x] the moved behaviour is covered by the same unit, golden and integration tests as before, re-pointed at the new package, with no test deleted; the PostgreSQL integration job and the compile-fail job are green on CI
- [x] the commit type is `feat!` with a `BREAKING CHANGE:` footer naming the old and new import path, so release-please puts it in the 0.4.0 notes

## Evidence

- Red first: ci-agent run 42585862491 on the throwaway branch worktree-agent-adffe7ae4d2ad2c57-red (headSha 952ca29, since deleted). It carried only timeop/compilefail_test.go, its fixtures and a doc-only timeop/doc.go (the commit gate refuses a test-only package), with the old root code intact. Unit and PostgreSQL integration failed with `TestCompileFailTimeExpressions: moved compiled; want a compile error`.
- Green: ci-agent run 42589630580, headSha 7a8fcd7 (code), all nine jobs success: lint, unit, apidiff-docs, fuzz-smoke, bench-module, integration (postgres), integration-mysql mysql84, mysql97 and mariadb. The runs 42585832666 and 42587835192 on earlier commits failed on a missing fixture, a nested Typed in IsBound, a one-column fixture and an unused helper; all fixed.
- Core search: no root file outside expr_time*.go calls `Now()` or `CurrentTimestamp()` (grep `(^|[^.A-Za-z_])Now\(\)` over root *.go found none); the core uses time.Now.
- Criterion 1: the golden tests (TestExtractGolden, interval, Now, DateTrunc cases) moved unchanged in SQL to timeop/timeop_test.go and pass in unit.
- Criterion 2: expr_time.go and its three test files are deleted; the JoinSQL tests that shared them moved to raw_join_test.go and raw_join_integration_test.go. internal/sqlshared/expr_integration_test.go now calls timeop.*. Docs, llms.txt and ai-guide updated.
- Criterion 3: timeop imports momo and dialect only; the one core addition is `momo.IsBound` (tested by TestIsBound). `feature timeop` is in packages.txt; internal/repocheck passes in unit.
- Criterion 4: internal/repocheck/testdata/moves/timeop.txt (all identifiers and constants) and matching rows in docs/upgrading.md; TestUpgradingCoversMoves passes in unit.
- Criterion 5: docs/ai-guide.md, docs/capabilities.md and llms.txt name timeop.*; apidiff-docs job success; timeop/example_test.go adds checked examples.
- Criterion 6: all tests kept (masking test in timeop_sensitive_test.go, integration tests in timeop_integration_test.go, SQLite month overflow wording kept in AddInterval and capabilities.md); the PostgreSQL integration job and unit job (which runs the compile-fail test) are green.
- Criterion 7: commit d7be694 is `feat(timeop)!:` with a BREAKING CHANGE footer.
