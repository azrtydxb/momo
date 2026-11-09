# Move advisory locks to momo/advisory as free functions

Status: done
Created: 2026-11-08
Epic: momo-14-structure-and-docs
Sprint: 006-sprint-6-split-the-root-package-into-subpackages-and-keep

## Description

Move `advisory.go` out of the root package into `github.com/azrtydxb/momo/advisory`. Identifiers moved: the eight methods `DB.AdvisoryXactLock`, `DB.AdvisoryXactLockString`, `DB.TryAdvisoryXactLock`, `DB.TryAdvisoryXactLockString` and the same four on `Tx`. A method cannot move off `DB` or `Tx`, so they become free functions taking a `momo.Querier`: `advisory.XactLock`, `advisory.XactLockString`, `advisory.TryXactLock`, `advisory.TryXactLockString`. They only call `Scalars` and `Exec`, which are exported. A call with a `DB` outside a transaction keeps returning the same error as the old method (the locks are transaction-scoped). Breaking for users: every call site and import changes and there is no alias bridge, because `advisory` imports `momo`. Detail and the resolution of blocking methods are in section 3 of `.procoder/research/package-split.md`. Depends on stories export-expression-extension-api and move-arrayop-package.

## Acceptance criteria

- [x] the advisory-lock integration test shows a second connection blocked until the first transaction commits, through the free functions, on PostgreSQL
- [x] every file listed in the description is deleted from the root package and its tests move with it; a search for each moved identifier in non-test root files, `examples/`, `internal/` and `docs/` finds no old reference (CI step output)
- [x] the new package imports only exported API of `momo` and no unexported name; the architecture test from story architecture-test-internal-and-subpackages passes with the package in its list
- [x] `internal/repocheck/testdata/moves/<package>.txt` lists every old and new identifier pair, and `docs/upgrading.md` has a row for each (the moves check from story upgrade-guide-for-the-breaking-imports passes)
- [x] every user-facing doc line and code block that named a moved identifier is updated in `README.md`, `docs/*.md`, `llms.txt` and `examples/`; the docs code-block test (momo-13 story 255) is green or, until it lands, each changed block is compiled by an example test in the new package
- [x] the moved behaviour is covered by the same unit, golden and integration tests as before, re-pointed at the new package, with no test deleted; the PostgreSQL integration job and the compile-fail job are green on CI
- [x] the commit type is `feat!` with a `BREAKING CHANGE:` footer naming the old and new import path, so release-please puts it in the 0.4.0 notes

## Evidence

- Red: ci-agent run 42600508638 on throwaway branch worktree-agent-ae15f3e2b98b26a59-red (sha 3dacf81, branch deleted): TestCompileFailAdvisory failed with the old methods in place.
- Green: ci-agent run 42600832614, headSha c4edde2, all nine jobs success, including integration (postgres) with TestXactLockContention and TestXactLockBlocksUntilCommit, and the unit job with the compile-fail, moves/upgrading, prefix and architecture checks.
- advisory.go deleted from root; unit and contention tests moved to advisory/; docs (ai-guide, capabilities, upgrading) updated; no old identifier left outside upgrading.md and the moves file.
- No core export added: the functions use Exec, Scalars, Dialect and a *momo.Tx type assertion.
- Review change: the four functions take `*momo.Tx` instead of `momo.Querier`, so a `*momo.DB` is a compile error (compile-fail program `dbarg`); a nil tx returns ErrValidation (unit test).
