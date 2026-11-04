# momo-03 Task 13: Pool statistics track acquisition (S-90)

Status: open
Created: 2026-11-04
Epic: momo-03-drivers-postgresql
Sprint: -

## Description

Implements plan momo-03-drivers Task 13: Pool statistics track acquisition (S-90) (`.procoder/plans/momo-03-drivers.md:1581`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-56, S-90. Files: `driver/pgxdriver/pin_test.go`.

## Acceptance criteria

- [ ] `TestPoolStatsReflectAcquisition` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-56] `TestStreamingBoundedMemory` iterates one million rows with Each and asserts heap growth stays under a fixed bound; fails if rows are buffered.
- [ ] [S-90] `TestPinnedConn` asserts session state set on a pinned connection is visible to later statements on it and the pool stats reflect acquisition; fails if statements hop connections.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

