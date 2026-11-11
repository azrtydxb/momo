# pgxdriver has no LISTEN/NOTIFY listener

Status: done 2026-11-11
Created: 2026-11-11
Epic: -
Sprint: 009-fix-every-open-github-issue-86-to-108-the-kuvryn-orbit-port
Type: bug
Severity: s2

## Description

GitHub issue #102, reported by the Kuvryn Orbit port. momo has no notification
API, so Orbit's SSE hub must keep a raw pgx `LISTEN` loop with its own
reconnect logic beside momo.

Reproduce: look for any way to receive or send a `NOTIFY` through
`driver/pgxdriver`; there is none.

Observed: no API. Expected: `pgxdriver.Listen` returns a `*Listener` on a
dedicated non-pooled connection with `Wait`, `Close`, `OnReconnect` and
`WithBackoff`, channel names validated and quoted, and `pgxdriver.Notify`
sending with `pg_notify`.

Root cause: the feature was never built; the driver only offers pooled
connections, which cannot hold a LISTEN session through PgBouncer in
transaction mode.

## Acceptance criteria

- [x] a regression test pins the fix: red before the change, green after
- [x] a notification sent in a transaction is delivered at commit and not at rollback
- [x] the longest payload PostgreSQL accepts (7999 bytes) is delivered, and 8000 is rejected by the server
- [x] after `pg_terminate_backend` on the listener's backend, `OnReconnect` runs and the listener receives again
- [x] `Close` ends a blocked `Wait` with `ErrListenerClosed`
- [x] channel names are validated and quoted; `WithBackoff` arguments are validated
- [x] docs: `docs/listen-notify.md`, nav entry, and a 0.8.0 section in `docs/upgrading.md`; `docs/reference.md` regenerated from CI if the run produces a change

## Evidence

- Red: run 42978065910 on worktree-agent-red-102 (tests only, commit ce677f5) failed to compile: `driver/pgxdriver/listen_test.go:16:13: undefined: validateChannel`, `listen_test.go:45:15: undefined: Listen`. Branch deleted.
- Green: ci-agent run 42985151956, headSha 777efc07793cd52294abc0377ee3dd23e842a470, all 9 jobs success (unit, integration postgres, mysql84, mysql97, mariadb, lint, fuzz-smoke, bench-module, apidiff-docs). The Listener integration tests run in the postgres job.
- The first attempt (run 42978625652) failed only on docs checks (code blocks, page index link, status prose); fixed in 777efc0.
- Docs: docs/listen-notify.md (new), docs/README.md index, mkdocs.yml nav, docs/upgrading.md 0.8.0 section. The run produced no regenerated-docs artifact and apidiff-docs passed, so docs/reference.md needed no change.
- Review follow-up (Wait keeps its connection when its ctx ends, redial pacing, last connect error in Wait's error, Close doc, redaction note): red run 42989866939 on worktree-agent-red-102 (commit 5e41e97) failed `TestListenWaitDeadlineThenResume` (`listen_integration_test.go:270: timed out waiting for notification sent between Waits`) and `TestListenWaitErrorNamesLastConnectError` (`listen_integration_test.go:313: Wait error "context deadline exceeded" does not name the last connect error`). `TestListenCloseDuringOnReconnectClosesConnection` already passed on the old code, so it is a guard, not a red test. Green: ci-agent run 42989988131, headSha ea77e4122c02bbc344c74f69da76c11fc134abf2, success.
