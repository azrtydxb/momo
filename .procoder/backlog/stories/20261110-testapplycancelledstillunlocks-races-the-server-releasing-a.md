# TestApplyCancelledStillUnlocks races the server releasing a discarded session's lock

Status: done 2026-11-10
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

- [x] a regression test pins the fix: red before the change, green after
- [x] the failing branch of `unlock` (unlock, unlock_all or discard) is named
      with CI evidence before the fix is chosen
- [x] `TestApplyCancelledStillUnlocks` passes on three CI runs of the same SHA
- [x] if the release is asynchronous by design, the `Lock` doc comment says so

## Evidence

Root cause, confirmed on CI (throwaway branch worktree-agent-probe-unlock,
logging each branch of the unlock closure, since deleted):

- Run 42903783313 (TestApplyCancelledStillUnlocks, -race, -count=20) and run
  42904226834 (-count=300 under CPU stress, 3 busy loops per core): in every
  iteration (320 of 320) the cancelled apply logged "pg_advisory_unlock FAILED:
  pgxdriver: pinned connection already released", then "pg_advisory_unlock_all
  err=... already released" and the "DISCARD branch taken" line.
- The connection was already discarded earlier: the cancelled session is
  broken, so Setup's reset (RESET lock_timeout) fails and calls discard, which
  closes the connection. The unlock that runs next finds the pinned connection
  released, and its discard is a no-op. So the lock is freed only when the
  PostgreSQL backend exits after the close handshake, asynchronously with Apply
  returning. This is by design: no unlock statement can run on a destroyed
  session, and a later apply that meets the lock gets ErrLocked with the holder
  named. The unlock code is not broken.
- Red before: PR #80 run 42888685470, job integration (1.26, 0.8.7-pg16,
  postgres), failed the old test ("lock still held after a cancelled apply:
  held by pid=4023"). The race did not reproduce in 320 probe iterations, so
  the old test could not be made to fail on demand; this one real failure is
  the red evidence.

Fix (commit db04174, final code SHA 32f76ff): the test polls Lock for up to 10s
while it returns ErrLocked, and the Lock doc comment now states that release
after a discarded session is asynchronous.

Green: ci-agent run 42907768963 on SHA 32f76ff203064e3991d87af737199e54366d3fe9,
job integration (postgres) passed in attempt 1 (job 130150331889), attempt 2
(130161732915) and attempt 3 (130170655533); every other job of attempt 1 was
green too.
