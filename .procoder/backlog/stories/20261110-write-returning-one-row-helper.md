# Writes: a one-row helper for INSERT/UPDATE/DELETE ... RETURNING

Status: done 2026-11-10
Created: 2026-11-10
Epic: -
Sprint: 008-sprint-8-fix-the-kuvryn-orbit-port-issues-73-to-79-and-the
Type: story

## Description

A write with RETURNING is read through `Rows(ctx)` and a Next/ScanModel/Close loop; there is no counterpart of `One`. Add `One(ctx) (T, error)` to the insert, update and delete queries, returning `ErrNotFound` when no row came back, so `ON CONFLICT DO NOTHING` or a guarded UPDATE is distinguishable. GitHub issue #78.

## Acceptance criteria

- [x] the tests are shown failing on CI before the change
- [x] `InsertQuery.One`, `UpdateQuery.One` and `DeleteQuery.One` return the RETURNING row, and `ErrNotFound` when none came back; tests cover a DO NOTHING insert and a guarded update matching no row on every dialect that supports RETURNING
- [x] more than one returned row is an error, or the docs say only the first is read

## Evidence

- RED on CI: throwaway branch worktree-agent-ab6d-one-red (commit 8e7e47c, tests only, no implementation), ci-agent run 42903952859 failed: `momo.InsertQuery[suite.Document] has no field or method One` (and Update/Delete) in write_returning_test.go and write_returning_integration_test.go. Branch deleted afterwards.
- GREEN on CI: ci-agent run 42910855348 (headSha 36a94e3), all jobs success. Covering tests: TestWriteOneReturnsTheRow, TestWriteOneNotFound (DO NOTHING insert, guarded update, delete matching nothing), TestWriteOneTooManyRows (multi-row update and delete give ErrTooManyRows; under WithImplicitTx both rows survive), run on postgres and sqlite via testopen.Each; TestWriteOneUnsupportedOnMySQL (unit, fakedriver MySQL: ErrUnsupported on all three).
- Design: One without Returning adds ReturningAll; more than one row is ErrTooManyRows (rolled back under WithImplicitTx); MariaDB supports insert One only (InsertReturning).
- Docs: docs/upgrading.md (0.7.0), docs/ai-guide.md, docs/capabilities.md; docs/reference.md from the CI regenerated-docs artifact of run 42907527999 (same implementation).
- Review fix round (integrator-verified, commits f47b144, 9dcfb1f, 7e8e55a): InsertQuery.One refuses a multi-row Models insert with ErrValidation before any statement runs (TestWriteOneRefusesBulkInsert; red on throwaway branch worktree-agent-ab6d-one-red2, run 42916236695, which got ErrNotFound instead; branch deleted). TestWriteOneTooManyRowsOnPlainDB pins that a plain *DB has already changed both rows when ErrTooManyRows comes back, and the doc comments and docs say to run One inside db.Tx. TestWriteOneRunsHooks and TestWriteOneKeepsTheFromGuard pin hooks and the From/Using guard through One (these passed before the fix; they pin existing behaviour). MariaDB wording: UPDATE and DELETE RETURNING are not implemented in momo. GREEN: ci-agent run 42920919562 on 7e8e55aba780bad13f36595da5c23d1814ee2bc2, all jobs success; docs/reference.md from the regenerated-docs artifact of run 42916384422.
