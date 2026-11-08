# Move advisory locks to momo/advisory as free functions

Status: open
Created: 2026-11-08
Epic: momo-14-structure-and-docs
Sprint: 006-sprint-6-split-the-root-package-into-subpackages-and-keep

## Description

Move `advisory.go` out of the root package into `github.com/azrtydxb/momo/advisory`. Identifiers moved: the eight methods `DB.AdvisoryXactLock`, `DB.AdvisoryXactLockString`, `DB.TryAdvisoryXactLock`, `DB.TryAdvisoryXactLockString` and the same four on `Tx`. A method cannot move off `DB` or `Tx`, so they become free functions taking a `momo.Querier`: `advisory.XactLock`, `advisory.XactLockString`, `advisory.TryXactLock`, `advisory.TryXactLockString`. They only call `Scalars` and `Exec`, which are exported. A call with a `DB` outside a transaction keeps returning the same error as the old method (the locks are transaction-scoped). Breaking for users: every call site and import changes and there is no alias bridge, because `advisory` imports `momo`. Detail and the resolution of blocking methods are in section 3 of `.procoder/research/package-split.md`. Depends on stories export-expression-extension-api and move-arrayop-package.

## Acceptance criteria

- [ ] the advisory-lock integration test shows a second connection blocked until the first transaction commits, through the free functions, on PostgreSQL
- [ ] every file listed in the description is deleted from the root package and its tests move with it; a search for each moved identifier in non-test root files, `examples/`, `internal/` and `docs/` finds no old reference (CI step output)
- [ ] the new package imports only exported API of `momo` and no unexported name; the architecture test from story architecture-test-internal-and-subpackages passes with the package in its list
- [ ] `internal/repocheck/testdata/moves/<package>.txt` lists every old and new identifier pair, and `docs/upgrading.md` has a row for each (the moves check from story upgrade-guide-for-the-breaking-imports passes)
- [ ] every user-facing doc line and code block that named a moved identifier is updated in `README.md`, `docs/*.md`, `llms.txt` and `examples/`; the docs code-block test (momo-13 story 255) is green or, until it lands, each changed block is compiled by an example test in the new package
- [ ] the moved behaviour is covered by the same unit, golden and integration tests as before, re-pointed at the new package, with no test deleted; the PostgreSQL integration job and the compile-fail job are green on CI
- [ ] the commit type is `feat!` with a `BREAKING CHANGE:` footer naming the old and new import path, so release-please puts it in the 0.4.0 notes

## Evidence
