# TestApplyCancelledStillUnlocks races the server releasing a discarded session's lock

Status: open
Created: 2026-11-10
Epic: -
Sprint: 008-sprint-8-fix-the-kuvryn-orbit-port-issues-73-to-79-and-the
Type: bug
Severity: s3

## Description

Reproduction: run `make test-integration-postgres` on CI. The v0.6.0 PR #80 run
42888685470, job `integration (1.26, 0.8.7-pg16, postgres)`, failed
`TestApplyCancelledStillUnlocks` (migrate/engine_apply_test.go:459) with "lock
still held after a cancelled apply: migrate: migration lock is held by another
process: held by pid=4023". The same job in the parallel push run passed. The
test dates from sprint 5, and nothing in #80 touches the migrate lock.

Observed vs expected: the test cancels an apply while `pg_sleep(30)` runs, then
takes the lock on a fresh connection right away. A different backend (pid 4023)
still held the lock, so the unlock in migrate/postgres/lock.go did not succeed
on the cancelled session. The likely path: the cancellation broke the
connection, `pg_advisory_unlock` and `pg_advisory_unlock_all` both failed, and
`discard` closed the connection. A session lock on a discarded connection is
released only when its backend exits, which happens shortly after `Apply`
returns. The test checks immediately and sometimes wins that race.

Expected: the lock is released by the time a later apply needs it, and the test
does not flake. Either the test waits a bounded time for a discarded session to
end, or the unlock path waits for that session to end before it returns.

Part of the fix is deciding which. If the path is confirmed, a test that waits
is enough, because a later apply that meets the lock gets `ErrLocked` with the
holder named and can retry. Confirm the path on CI first: log which branch
of `unlock` ran, on a throwaway branch.

## Acceptance criteria

- [ ] a regression test pins the fix: red before the change, green after
- [ ] the failing branch of `unlock` (unlock, unlock_all or discard) is named
      with CI evidence before the fix is chosen
- [ ] `TestApplyCancelledStillUnlocks` passes on three CI runs of the same SHA
- [ ] if the release is asynchronous by design, the `Lock` doc comment says so

## Evidence
