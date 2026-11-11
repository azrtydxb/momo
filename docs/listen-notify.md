# LISTEN and NOTIFY

`pgxdriver.Listener` receives PostgreSQL `LISTEN`/`NOTIFY` notifications on
one dedicated connection and reconnects when that connection is lost.
`pgxdriver.Notify` sends one. Both are PostgreSQL only.

The listener is not part of a `*momo.DB` or a pool. A `LISTEN` session lives
as long as its connection, so it cannot go through PgBouncer in transaction
mode: connect to the server directly, or to PgBouncer in session mode.

## Listen

```go
cfg, err := pgx.ParseConfig(url)
if err != nil {
	return err
}
l, err := pgxdriver.Listen(ctx, cfg, []string{"orders", "invoices"},
	pgxdriver.OnReconnect(func() { resync() }),
	pgxdriver.WithBackoff(200*time.Millisecond, 15*time.Second),
)
if err != nil {
	return err
}
defer l.Close(context.Background())

for {
	n, err := l.Wait(ctx)
	if err != nil {
		return err // ctx ended, or errors.Is(err, pgxdriver.ErrListenerClosed)
	}
	handle(n.Channel, n.Payload, n.PID)
}
```

`Listen` connects and runs `LISTEN` before it returns, so a nil error means the
listener is subscribed. It copies the config. `Wait` returns the next
`Notification` (`Channel`, `Payload`, `PID` of the notifying backend). Call
`Wait` from one goroutine at a time; `Close` may be called from any
goroutine, ends a blocked `Wait` with `ErrListenerClosed`, and is safe to call
twice.

## Reconnecting

When the connection drops (a restart, a failover, `pg_terminate_backend`),
`Wait` reconnects inside the same call, re-issues `LISTEN` for every channel,
runs the `OnReconnect` function and goes on waiting. The first attempt is
immediate. After a failed attempt the delay starts at the `WithBackoff`
minimum and doubles up to its maximum (default 100ms to 10s). `Wait` keeps
trying until it connects, `ctx` ends or `Close` runs.

Notifications sent while the listener was disconnected are lost: PostgreSQL
does not queue them for absent listeners. Treat every `OnReconnect` call as
"something may have been missed" and re-read whatever the notifications
signal. `OnReconnect` runs on the goroutine that called `Wait`, before `Wait`
returns anything from the new connection, so keep it short.

When the `ctx` given to `Wait` ends, `Wait` returns its error and drops the
connection, because a read that was interrupted leaves it unusable. The next
`Wait` reconnects and calls `OnReconnect`. Pass a context that lives as long
as the loop, not a short per-call timeout.

## Notify

```go
err := pgxdriver.Notify(ctx, tx, "orders", `{"id":42}`)
```

`Notify` runs `SELECT pg_notify($1, $2)` on any `momo.Querier`. Inside a
transaction PostgreSQL delivers the notification at commit and drops it on
rollback; on a `*momo.DB` it is delivered at once.

## Channels and payloads

Channel names are validated (not empty, no NUL byte, at most 63 bytes, the
PostgreSQL identifier limit) and quoted as identifiers, so `LISTEN` and
`pg_notify` agree on the exact name, case included. Notifications are
per database, not per schema.

PostgreSQL rejects a payload of 8000 bytes or more, so the longest payload is
7999 bytes. `Notify` returns the server's error for a longer one. For larger
data, notify an ID and read the row.
