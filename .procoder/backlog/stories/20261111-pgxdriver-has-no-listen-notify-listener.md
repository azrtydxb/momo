# pgxdriver has no LISTEN/NOTIFY listener

Status: open
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

- [ ] a regression test pins the fix: red before the change, green after
- [ ] a notification sent in a transaction is delivered at commit and not at rollback
- [ ] the longest payload PostgreSQL accepts (7999 bytes) is delivered, and 8000 is rejected by the server
- [ ] after `pg_terminate_backend` on the listener's backend, `OnReconnect` runs and the listener receives again
- [ ] `Close` ends a blocked `Wait` with `ErrListenerClosed`
- [ ] channel names are validated and quoted; `WithBackoff` arguments are validated
- [ ] docs: `docs/listen-notify.md`, nav entry, and a 0.8.0 section in `docs/upgrading.md`; `docs/reference.md` regenerated from CI

## Evidence

<!-- Filled at close time. -->
