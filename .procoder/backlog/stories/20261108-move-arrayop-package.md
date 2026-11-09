# Move array predicates to momo/arrayop (pilot move)

Status: done
Created: 2026-11-08
Epic: momo-14-structure-and-docs
Sprint: 006-sprint-6-split-the-root-package-into-subpackages-and-keep

## Description

First of the leaf moves and the pilot, because `array_ops.go` is the smallest (75 lines) and proves the pattern, the `moves/` file, the docs edit and the compile-fail test before the larger moves. New import path `github.com/azrtydxb/momo/arrayop`; identifiers moved: `ArrayContains`, `ArrayContainedBy`, `ArrayOverlaps`, `AnyOf`, with the unexported `pgArrayPred` and `boundSlice`. Names in the new package drop the prefix: `arrayop.Contains`, `arrayop.ContainedBy`, `arrayop.Overlaps`, `arrayop.AnyOf`. The predicate type embeds `momo.Ops[bool]` and uses `ArgFor`, `RenderSensitive`, `Unsupported` and `FlagsOf` from the extension API. Breaking for users: every call site and import changes; no alias is possible because `arrayop` imports `momo`. Depends on story export-expression-extension-api.

## Acceptance criteria

- [x] every file listed in the description is deleted from the root package and its tests move with it; a search for each moved identifier in non-test root files, `examples/`, `internal/` and `docs/` finds no old reference (CI step output)
- [x] the new package imports only exported API of `momo` and no unexported name; the architecture test from story architecture-test-internal-and-subpackages passes with the package in its list
- [x] `internal/repocheck/testdata/moves/<package>.txt` lists every old and new identifier pair, and `docs/upgrading.md` has a row for each (the moves check from story upgrade-guide-for-the-breaking-imports passes)
- [x] every user-facing doc line and code block that named a moved identifier is updated in `README.md`, `docs/*.md`, `llms.txt` and `examples/`; the docs code-block test (momo-13 story 255) is green or, until it lands, each changed block is compiled by an example test in the new package
- [x] the moved behaviour is covered by the same unit, golden and integration tests as before, re-pointed at the new package, with no test deleted; the PostgreSQL integration job and the compile-fail job are green on CI
- [x] the commit type is `feat!` with a `BREAKING CHANGE:` footer naming the old and new import path, so release-please puts it in the 0.4.0 notes

## Evidence

Implementation commit cf3c8c2 (the story edit lands in a later docs-only commit). All results are from GitHub Actions; nothing was run locally.

- Green: ci-agent run 42578948966, headSha cf3c8c2cfffb17ea54d67581f436b00015a4eae9, all nine jobs success (unit, lint, apidiff-docs, fuzz-smoke, bench-module, integration postgres, mysql84, mysql97, mariadb).
- Red first: ci-agent run 42578940857 on the throwaway branch worktree-agent-a37c909eec1e6e2f5-red (headSha 1870cc1, since deleted). It carried only the new compile-fail test, its fixtures, the moves file and the upgrade rows, with the old root code intact. Unit and integration jobs failed with `TestCompileFailArrayPredicatesMoved: arrayop_moved compiled; want a compile error`. The new-package tests could not be part of the red commit: the commit gate refuses a package with only test files (golangci-lint "no non-test Go files").
- Criterion 1: array_ops.go, array_ops_test.go and array_ops_integration_test.go are gone from the root (renamed into arrayop/). A grep of `ArrayContains|ArrayContainedBy|ArrayOverlaps|pgArrayPred|boundSlice|momo.AnyOf` over *.go, *.md and *.txt outside .git and .procoder finds hits only in docs/upgrading.md, testdata/moves/arrayop.txt, the compile-fail fixture and test, the repocheck self-test and arrayop/ itself. README.md, llms.txt and examples/ never named the identifiers.
- Criterion 2: arrayop imports momo and dialect only and calls `Ops`, `NewOps`, `Builder.Dialect`, `Builder.WriteString`, `Builder.Unsupported`, `Builder.RenderSensitive`, `Builder.ArgFor` and `FlagsOf`, all exported. `feature arrayop` is in internal/repocheck/testdata/packages.txt; `internal/repocheck` is ok in the unit job of run 42578948966.
- Criterion 3: internal/repocheck/testdata/moves/arrayop.txt (four pairs) and four rows in docs/upgrading.md; TestUpgradingCoversMoves passes in the same job.
- Criterion 4: docs/ai-guide.md (array predicates paragraph) and docs/capabilities.md (matrix row and the argument masking bullet) now name arrayop.*; apidiff-docs job success. No Go code block named the identifiers; arrayop/example_test.go adds a runnable Example with checked output.
- Criterion 5: the three test files moved with a rename and kept every test (masking, golden, unsupported off PostgreSQL, live, hostile values, integer columns, nil slice). `ok github.com/azrtydxb/momo/arrayop` in unit, race and all four integration jobs; the compile-fail job is green.
- Criterion 6: commit cf3c8c2 is `feat(arrayop)!:` with a `BREAKING CHANGE:` footer naming the old and new identifiers and the import path.

Pilot notes for the next moves:

- The commit gate refuses a package that has only test files, so a red commit cannot contain the new package's tests. Use the root compile-fail test as the red.
- Moved tests need their own copies of unexported root fixtures (`unitTally`, `typedDB`, `kvRow`); the arrayop tests have a fixtures_test.go.
- Prettier re-pads the whole identifier table in docs/upgrading.md when a longer name lands, so parallel moves will conflict on that table; take both sides and re-run the formatter.
