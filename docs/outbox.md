# Transactional outbox

A service often has to publish an event together with a data change: an order
is marked paid, and a message goes to a broker. Writing to the database and
then to the broker is two steps, and a crash between them loses the message or
sends one for a change that never committed. The transactional outbox removes
the gap. The event is written into an outbox table in the same transaction as
the data, and a relay publishes it afterwards.

The `outbox` package has two parts. `outbox.Outbox.Add` writes an event inside
the caller's transaction. `outbox.Relay` reads the committed events and hands
them to a `outbox.Publisher` that the application implements. Momo ships no
broker client: the publisher is your code for Kafka, NATS, SQS or an HTTP
endpoint.

It runs on PostgreSQL, SQLite, MySQL and MariaDB; see
[Dialects](#dialects).

## Declare the table

The outbox table is an ordinary model, so `momo migrate diff` creates it and
your other tooling sees it. Copy the struct below into your models package and
run `momo generate` and `momo migrate diff`. The default table name is
`outbox_events`; `outbox.WithTable` selects another, and the model's table tag
must then match.

```go
package main

import (
	"fmt"
	"time"

	"github.com/azrtydxb/momo"
)

// OutboxEvent is one event that waits to be published.
type OutboxEvent struct {
	momo.Model  `momo:"table:outbox_events"`
	ID          int64      `momo:"column:id,pk,identity"`
	Topic       string     `momo:"column:topic,size:255"`
	Key         *string    `momo:"column:key,size:255"`
	Payload     []byte     `momo:"column:payload"`
	Headers     *string    `momo:"column:headers,type:text"`
	CreatedAt   time.Time  `momo:"column:created_at"`
	AvailableAt time.Time  `momo:"column:available_at"`
	Attempts    int32      `momo:"column:attempts,default:0"`
	PublishedAt *time.Time `momo:"column:published_at"`
	DeadAt      *time.Time `momo:"column:dead_at"`
	LastError   *string    `momo:"column:last_error,type:text"`
}

// MomoConfig declares the two indexes the relay and the retention cleanup read
// through. They are plain indexes, not partial ones, because MySQL and MariaDB
// have no partial indexes; a NULL published_at or dead_at is searched as a key
// value.
func (e *OutboxEvent) MomoConfig(c *momo.Config) {
	c.Index("outbox_events_due_idx", &e.PublishedAt, &e.DeadAt, &e.AvailableAt)
	c.Index("outbox_events_key_idx", &e.Key, &e.PublishedAt, &e.ID)
}

func main() {
	fmt.Println("the outbox model is declared")
}
```

What each column is for:

| Column         | Meaning                                                                                  |
| -------------- | ---------------------------------------------------------------------------------------- |
| `id`           | Identity key. It orders the events of a key and is the deduplication key for consumers.  |
| `topic`        | Where the event goes. Required.                                                          |
| `key`          | Orders events: the events of one key are published in order. NULL means no order.        |
| `payload`      | The message body.                                                                        |
| `headers`      | Optional metadata, a JSON object of strings.                                             |
| `created_at`   | When `outbox.Outbox.Add` ran.                                                            |
| `available_at` | The earliest time the event can be claimed: the retry time, or the end of a claim lease. |
| `attempts`     | How many delivery attempts were counted.                                                 |
| `published_at` | Set when the publisher accepted the event.                                               |
| `dead_at`      | Set when the last allowed attempt failed.                                                |
| `last_error`   | The text of the last publish error, cut to 1000 bytes.                                   |

The table holds only what the relay needs. The two indexes keep the claim and
the retention cleanup fast however many published rows are kept. On MySQL and
MariaDB `payload` is a `blob` (65,535 bytes at most) and `headers` is `text`;
change the column type in a migration if events are larger.

## Write an event with the data

`outbox.Outbox.Add` takes the `*momo.Tx` that writes the data. The parameter is
a transaction, not a `momo.Querier`, so passing a `*momo.DB` does not compile:
an event written outside the transaction would defeat the purpose. A nil
transaction and an empty topic fail with `momo.ErrValidation`.

```go
package main

import (
	"context"
	"log"
	"os"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/pgxdriver"
	"github.com/azrtydxb/momo/outbox"
)

func main() {
	ctx := context.Background()
	pool, err := pgxdriver.NewPool(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	db := momo.New(pgxdriver.New(pool))

	ob, err := outbox.New()
	if err != nil {
		log.Fatal(err)
	}
	err = db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
		if _, err := momo.Exec(ctx, tx, "UPDATE orders SET status = ? WHERE id = ?", "paid", 42); err != nil {
			return err
		}
		_, err := ob.Add(ctx, tx, outbox.Event{
			Topic:   "orders.paid",
			Key:     "order-42",
			Payload: []byte(`{"id":42}`),
			Headers: map[string]string{"trace": "4bf92f35"},
		})
		return err
	})
	if err != nil {
		log.Fatal(err)
	}
}
```

If the function returns an error, both the update and the event roll back.
`outbox.Outbox.Add` returns the id of the new row. On PostgreSQL it also runs
`SELECT pg_notify(table, '')` in the same transaction, which PostgreSQL
delivers at commit and drops on rollback; `outbox.WithoutNotify` turns that
off. The payload and the headers are bound as statement arguments, so a hook
opened with `momo.WithArgLogging` sees them.

## Run the relay

`outbox.Outbox.NewRelay` takes the `*momo.DB`, your publisher and options.
`outbox.Relay.Run` publishes until its context ends. In the program below the
relay wakes on `NOTIFY` through `outbox.WithListen`, and deletes events that were
published more than a week ago.

```go
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/pgxdriver"
	"github.com/azrtydxb/momo/outbox"
	"github.com/jackc/pgx/v5"
)

// broker is the application's publisher. Replace the log call with the
// broker's client and return its error.
type broker struct{}

func (broker) Publish(ctx context.Context, e outbox.Event) error {
	log.Printf("publish topic=%s key=%s id=%d attempt=%d bytes=%d", e.Topic, e.Key, e.ID, e.Attempt, len(e.Payload))
	return nil
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	dsn := os.Getenv("DATABASE_URL")
	pool, err := pgxdriver.NewPool(ctx, dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	db := momo.New(pgxdriver.New(pool))

	ob, err := outbox.New()
	if err != nil {
		log.Fatal(err)
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		log.Fatal(err)
	}
	relay, err := ob.NewRelay(db, broker{},
		outbox.WithListen(cfg),
		outbox.WithPollInterval(10*time.Second),
		outbox.WithMaxAttempts(8),
		outbox.WithBackoff(time.Second, time.Minute),
		outbox.WithRetention(7*24*time.Hour),
	)
	if err != nil {
		log.Fatal(err)
	}
	if err := relay.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
```

`outbox.Relay.Run` returns nil when its context ends. The relay can also be
driven by hand: `outbox.Relay.RunOnce` claims and publishes one batch and returns
how many events it claimed, which suits a cron job or a test. `outbox.Relay.Wake`
makes an idle `Run` look at once, for example from the code that just committed
an event.

## How a batch is delivered

One statement claims a batch of due events. It moves each event's
`available_at` to the end of a lease (30 seconds by default) and counts an
attempt. The statement selects with `FOR UPDATE SKIP LOCKED`, so a second relay
skips the rows the first is claiming. The row locks end with that statement
(on MySQL and MariaDB with one short transaction). The events are then published one at a time, in id order, with no
transaction and no lock held, so a slow broker does not hold a row lock or a
connection. After each publish the relay records the outcome:

- Published: `published_at` is set.
- Failed: `last_error` is set and `available_at` moves to the retry time. The
  delay is the backoff base after the first failure and doubles with each
  failure up to the maximum (one second up to five minutes by default).
- Failed on the last allowed attempt (10 by default): `dead_at` is set. The
  event is not retried.

If a relay dies, its leases run out and the events are claimed again. A
batch must fit in the lease: the context given to the publisher ends at the end
of the lease, and the events of the batch that were not reached by then are
released, not counted as attempts. If a publisher panics, the relay treats it as
a failed attempt.

## Delivery guarantees

Delivery is at least once. An event is published twice when a relay crashes
after the publisher returned and before the relay marked the event, and when a
publish runs past its lease. Make consumers idempotent. `outbox.Event.ID` is the
same on every attempt and works as the deduplication key; `outbox.Event.Attempt`
says which attempt this is.

### Order

Events with the same non-empty key are published in the order they were added.
The relay claims an event only when every earlier event of its key is published,
so the head of each key is claimed and the rest wait. Consequences:

- A key advances by at most one event per batch. `outbox.Relay.Run` claims again
  at once while there is work, so a backlog drains without waiting for the poll
  interval.
- An event that fails holds back the later events of its key until it succeeds
  or dies, and a dead event holds them back until `outbox.Outbox.Requeue` brings
  it back or the row is deleted. This keeps the order; it also means one
  poisoned event can stall its key. Watch for dead events.
- Events of different keys, and events without a key, have no order between
  them.

Order is by `id`, and PostgreSQL assigns an `id` when the row is inserted, not
when the transaction commits. Two transactions that add events of the same key
at the same time can therefore commit in the opposite order. Serialize the
writers of one key, for example by locking the row of the entity the key names
(`SELECT ... FOR UPDATE`) before adding the event.

## Several relays

Run as many relays as you like, in one process or many, against one table. They
share the work and do not publish the same event, except in the at-least-once
cases above. The relays measure leases and backoff with the clock of the process
that claims, so the hosts need clocks that agree to within a small part of the
lease.

On SQLite there is one writer: each claim is one statement, so claims are
serialized. There is no `LISTEN` on SQLite, MySQL or MariaDB, so the relay polls
there, and `outbox.WithListen` fails with `momo.ErrUnsupported`.

## Dialects

| Dialect       | Claim                                                                                                      | Wake-up                |
| ------------- | ---------------------------------------------------------------------------------------------------------- | ---------------------- |
| PostgreSQL    | One `UPDATE ... RETURNING` over a `FOR UPDATE SKIP LOCKED` selection                                       | `LISTEN`, then polling |
| SQLite        | One `UPDATE ... RETURNING`; the single writer serializes claims                                            | Polling                |
| MySQL 8.0+    | A short `READ COMMITTED` transaction: `SELECT ... FOR UPDATE SKIP LOCKED`, `UPDATE` with the lease, commit | Polling                |
| MariaDB 10.6+ | The same transaction as MySQL                                                                              | Polling                |

MySQL and MariaDB have no `UPDATE ... RETURNING`, so the claim selects and
leases in one transaction and commits before anything is published: the
semantics are those of the PostgreSQL claim, with the same lease, retries,
order and at-least-once delivery. The relay asks the server for its version
when it first claims. A server older than MySQL 8.0 or MariaDB 10.6 has no
`SKIP LOCKED`, and `outbox.Relay.RunOnce` and `outbox.Relay.Run` return
`momo.ErrUnsupported`. `outbox.WithListen` on a database other than PostgreSQL
fails in `outbox.Outbox.NewRelay` with `momo.ErrUnsupported`. The remaining
statements are the same on every dialect, apart from the quoting of
identifiers and the retention delete, which MySQL writes as
`DELETE ... ORDER BY id LIMIT`.

## Waking the relay

Without `outbox.WithListen`, a relay finds new events when its poll interval
(one second by default) ends. On PostgreSQL, `outbox.Outbox.Add` sends a
notification at commit and `outbox.WithListen` makes the relay wait for it on
its own connection, using the listener described in
[LISTEN and NOTIFY](listen-notify.md): connect to the server directly, or to
PgBouncer in session mode. A notification that arrives while the relay is busy
is not lost; the relay looks again before it idles. A notification sent while
the listener was disconnected is lost, so the relay also looks at every
reconnect, and the poll interval stays as the fallback. With a listener you can
lengthen the poll interval, but do not make it infinite. If the listener cannot
connect, the relay reports the error, keeps polling and retries.

## Retention and dead events

Published rows stay in the table until you remove them.
`outbox.Outbox.Cleanup` deletes the rows published longer ago than a retention,
in chunks of 1000. `outbox.WithRetention` makes `outbox.Relay.Run` do that once
a minute. Cleanup never touches pending or dead events.

A dead event stays for an operator. Find it with
`SELECT id, topic, key, last_error FROM outbox_events WHERE dead_at IS NOT NULL`,
fix the cause, and call `outbox.Outbox.Requeue` with its ids: the event is
pending again with its attempts reset, and the later events of its key are
released. Deleting the row also releases the key, and drops the event.

## Observability

Every statement of `outbox.Outbox.Add` and the relay goes through the `*momo.DB`
or `*momo.Tx` it was given, so the hooks of [observability](observability.md)
see the claims, the marks and the cleanup like any other statement.
`outbox.WithObserver` adds one callback per delivery attempt with an
`outbox.Outcome`: the event, the publisher's error, whether the event became
dead and how long the publish took. `outbox.WithOnError` receives the errors the
relay handles on its own, such as a failed claim or a listener that cannot
connect; without it they go to `log/slog` as warnings. A useful alert is a
count of rows with `dead_at` set, and the age of the oldest row with
`published_at` unset.

```go
package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/pgxdriver"
	"github.com/azrtydxb/momo/outbox"
)

func main() {
	ctx := context.Background()
	pool, err := pgxdriver.NewPool(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	db := momo.New(pgxdriver.New(pool))
	ob, err := outbox.New()
	if err != nil {
		log.Fatal(err)
	}

	publish := outbox.PublisherFunc(func(ctx context.Context, e outbox.Event) error { return nil })
	relay, err := ob.NewRelay(db, publish,
		outbox.WithObserver(func(oc outbox.Outcome) {
			if oc.Err != nil {
				log.Printf("event %d attempt %d failed (dead=%v): %v", oc.Event.ID, oc.Event.Attempt, oc.Dead, oc.Err)
			}
		}),
		outbox.WithOnError(func(err error) { log.Printf("outbox: %v", err) }),
	)
	if err != nil {
		log.Fatal(err)
	}
	if _, err := relay.RunOnce(ctx); err != nil {
		log.Fatal(err)
	}

	// Remove what was published more than a day ago, then bring dead events 7
	// and 9 back.
	if _, err := ob.Cleanup(ctx, db, 24*time.Hour); err != nil {
		log.Fatal(err)
	}
	if _, err := ob.Requeue(ctx, db, 7, 9); err != nil {
		log.Fatal(err)
	}
}
```

## Limits

- MySQL needs 8.0 and MariaDB 10.6 for `SKIP LOCKED`.
- The table name is one identifier of at most 63 letters, digits and
  underscores. `outbox.New` rejects anything else with `momo.ErrValidation`.
- Delivery is at least once and order holds per key only, as described above.
- A batch is at most 1000 events. The PostgreSQL notification carries no
  payload: it only tells the relay to look.
