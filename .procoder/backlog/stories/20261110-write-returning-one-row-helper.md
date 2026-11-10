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
