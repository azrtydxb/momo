# migrate: intermittent integration failure in ./migrate

Status: open
Created: 2026-11-05

## Description

On 2026-11-05, right after merging story 087, one run of
`go test -tags integration -race -count=1 ./migrate/...` failed after 43s
(a clean run takes about 7s); the output was not captured. Thirteen later runs,
including one with -count=8, passed. During the story 087 review,
`TestApplyCancelledStillUnlocks` also failed once and the reviewer attributed it
to contention. The long duration suggests a test waiting on an advisory lock or
a lock timeout. Done means the failing test is identified and the cause is fixed,
not retried away.

## Acceptance criteria

- [ ] The failing test is identified from a captured run (loop the package with -count and -v into a file until it reproduces)
- [ ] The root cause is named in this task (for example a lock left held by a cancelled test, or a shared-name collision between tests)
- [ ] The fix makes `go test -tags integration -race -count=20 ./migrate/` pass with no failures

## Evidence

