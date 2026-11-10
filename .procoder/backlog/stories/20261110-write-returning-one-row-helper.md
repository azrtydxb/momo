# Writes: a one-row helper for INSERT/UPDATE/DELETE ... RETURNING

Status: open
Created: 2026-11-10
Epic: -
Sprint: 008-sprint-8-fix-the-kuvryn-orbit-port-issues-73-to-79-and-the
Type: story

## Description

A write with RETURNING is read through `Rows(ctx)` and a Next/ScanModel/Close loop; there is no counterpart of `One`. Add `One(ctx) (T, error)` to the insert, update and delete queries, returning `ErrNotFound` when no row came back, so `ON CONFLICT DO NOTHING` or a guarded UPDATE is distinguishable. GitHub issue #78.

## Acceptance criteria

- [ ] the tests are shown failing on CI before the change
- [ ] `InsertQuery.One`, `UpdateQuery.One` and `DeleteQuery.One` return the RETURNING row, and `ErrNotFound` when none came back; tests cover a DO NOTHING insert and a guarded update matching no row on every dialect that supports RETURNING
- [ ] more than one returned row is an error, or the docs say only the first is read

## Evidence
