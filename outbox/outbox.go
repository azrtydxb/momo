// SPDX-License-Identifier: Apache-2.0

// Package outbox is a transactional outbox: a service writes an event in the
// same transaction as the data change it describes, and a relay publishes the
// event afterwards. Either both reach the database or neither does, so a crash
// between "the row was saved" and "the message was sent" cannot lose the event.
//
// Momo ships no broker client. The application implements Publisher for its
// broker (Kafka, NATS, SQS, an HTTP endpoint) and the relay calls it.
//
// # The table
//
// The outbox table is an ordinary model. Declare it in your own package, copy
// the struct from the documentation page for this package, and `momo migrate
// diff` creates it like any other table. The default name is DefaultTable;
// WithTable changes it. The columns are id, topic, key, payload, headers,
// created_at, available_at, attempts, published_at, dead_at and last_error.
//
// # Writing
//
// Outbox.Add takes the *momo.Tx that writes the data, so the event commits or
// rolls back with it. Passing a *momo.DB does not compile. On PostgreSQL Add
// also sends NOTIFY on a channel named after the table; PostgreSQL delivers it
// when the transaction commits and drops it on rollback.
//
// # Relaying
//
// Outbox.NewRelay returns a Relay. Relay.Run claims a batch of due events with
// one statement (FOR UPDATE SKIP LOCKED on PostgreSQL), which moves their
// available_at to the end of a lease and counts an attempt. The row locks last
// only for that statement (on MySQL and MariaDB, one short transaction): the
// events are published afterwards, one at a time, with no lock and no
// transaction held across the publish. A published event is marked with
// published_at. An event whose publish fails is retried after an exponential
// backoff with jitter, and becomes dead (dead_at) after the maximum number of
// attempts. A relay that crashes loses its lease, and its events are claimed
// again when the lease ends; the attempt that was running counts, so an event
// that keeps crashing its relay becomes dead instead of being claimed forever.
// Any number of relays, in any number of processes, can run on one table. The
// statements that record an outcome apply only while the relay still holds the
// claim; see ErrLeaseLost.
//
// # Guarantees
//
// Delivery is at least once. An event is published again when a relay crashes
// after the Publisher returned and before the event was marked, and when a
// publish outlives the lease. Publishers must tolerate a duplicate; Event.ID is
// the same on every attempt and can serve as the deduplication key.
//
// Events with the same non-empty Key are published in the order they were
// added, by id: an event is claimed only when every earlier event of its key
// is published. Events of different keys, and events without a key, have no
// order between them. A failing event holds back later events of its key until
// it succeeds, and a dead event holds them back until Outbox.Requeue brings it
// back or the row is removed. The order is the id order, and PostgreSQL assigns
// an id when the row is inserted, not when the transaction commits, so two
// transactions that add events of the same key at the same time can commit in
// the other order. Serialize the writers of one key, for example by locking the
// row of the entity the key names.
//
// Every time in the table comes from the Go clock of the process that writes
// it, not from the database: the relay measures leases and backoff, and Add,
// Requeue and Cleanup stamp created_at, available_at and the retention cutoff.
// The processes that share a table need clocks that agree to within a small
// part of the lease.
//
// # Dialects
//
// PostgreSQL, SQLite, MySQL and MariaDB are supported. On SQLite the relay
// polls, and the single-writer database serializes the claims. On MySQL and
// MariaDB, which have no UPDATE ... RETURNING, the claim is a short transaction
// that selects the due rows with FOR UPDATE SKIP LOCKED, leases them and
// commits before anything is published, so the semantics are those of the
// PostgreSQL claim; it needs MySQL 8.0 or MariaDB 10.6, and a relay on an older
// server fails with momo.ErrUnsupported. Only PostgreSQL has a LISTEN wake-up;
// the others poll.
package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
)

// DefaultTable is the table name an Outbox uses unless WithTable says
// otherwise.
const DefaultTable = "outbox_events"

// tableName is the form a table name must have. The name is put into SQL text
// between double quotes, so it is checked rather than escaped.
var tableName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,62}$`)

// Event is one event of the outbox.
type Event struct {
	// ID is the primary key of the row. Add returns it and the relay sets it;
	// it is the same on every attempt, so a consumer can deduplicate on it.
	ID int64
	// Topic names where the event goes: a topic, a queue, a subject. It is
	// required.
	Topic string
	// Key orders events: those with the same non-empty Key are published in the
	// order they were added. It is usually the id of the entity the event is
	// about. An empty Key means no order.
	Key string
	// Payload is the message body. It may be empty.
	Payload []byte
	// Headers are optional string metadata, stored as JSON.
	Headers map[string]string
	// Attempt is the number of the delivery attempt, 1 for the first. The relay
	// sets it; Add ignores it.
	Attempt int
}

// Publisher sends an event to the broker. The application implements it.
//
// Publish must return nil only once the broker has accepted the event. An
// error, or a panic, makes the relay retry the event after a backoff. The
// context ends when the relay stops or when the lease on the event ends; a
// Publisher should return soon after that. A Publisher is called from one
// goroutine at a time per Relay, but one Publisher value may be shared by
// several relays, so it must be safe for concurrent use.
type Publisher interface {
	Publish(ctx context.Context, e Event) error
}

// PublisherFunc adapts a function to a Publisher.
type PublisherFunc func(ctx context.Context, e Event) error

// Publish calls f.
func (f PublisherFunc) Publish(ctx context.Context, e Event) error { return f(ctx, e) }

// Outbox is the handle to one outbox table. It holds no connection and is safe
// for concurrent use.
type Outbox struct {
	table  string
	notify bool
	sql    map[dialect.Name]*statements
}

// Option configures an Outbox.
type Option func(*Outbox)

// WithTable sets the table name. It must be an identifier of at most 63 bytes
// made of letters, digits and underscores that does not start with a digit;
// anything else makes New fail with momo.ErrValidation. The default is
// DefaultTable.
func WithTable(name string) Option { return func(o *Outbox) { o.table = name } }

// WithoutNotify stops Add from sending NOTIFY on PostgreSQL. A relay then
// finds new events only by polling, or when Relay.Wake is called.
func WithoutNotify() Option { return func(o *Outbox) { o.notify = false } }

// New returns an Outbox. It fails with momo.ErrValidation for an invalid table
// name.
func New(opts ...Option) (*Outbox, error) {
	o := &Outbox{table: DefaultTable, notify: true}
	for _, opt := range opts {
		if opt != nil {
			opt(o)
		}
	}
	if !tableName.MatchString(o.table) {
		return nil, validation("outbox new", fmt.Sprintf("table name %q is not an identifier of at most 63 letters, digits and underscores", o.table))
	}
	o.sql = map[dialect.Name]*statements{}
	for _, n := range []dialect.Name{dialect.Postgres, dialect.SQLite, dialect.MySQL} {
		s := newStatements(n, o.table)
		o.sql[n] = &s
	}
	return o, nil
}

// Table returns the table name.
func (o *Outbox) Table() string { return o.table }

// Add writes e into the outbox inside tx and returns its id. The event becomes
// visible to the relay when tx commits and never appears if tx rolls back.
//
// tx must be the transaction that writes the data the event describes. A nil
// tx and an empty topic fail with momo.ErrValidation. On PostgreSQL Add also
// sends NOTIFY on the channel named after the table, unless WithoutNotify was
// given.
//
// The payload and headers are bound as statement arguments, so a hook opened
// with momo.WithArgLogging sees them.
func (o *Outbox) Add(ctx context.Context, tx *momo.Tx, e Event) (int64, error) {
	if tx == nil {
		return 0, validation("outbox add", "Add needs the *momo.Tx that writes the data; pass the tx of DB.Tx or DB.Begin")
	}
	d := tx.Dialect()
	if err := supported(d, "outbox add"); err != nil {
		return 0, err
	}
	if e.Topic == "" {
		return 0, validation("outbox add", "an event needs a topic")
	}
	var key, headers any
	if e.Key != "" {
		key = e.Key
	}
	if len(e.Headers) > 0 {
		b, err := json.Marshal(e.Headers)
		if err != nil {
			return 0, validation("outbox add", "headers: "+err.Error())
		}
		headers = string(b)
	}
	payload := e.Payload
	if payload == nil {
		payload = []byte{}
	}
	now := stamp(d, time.Now())
	st := o.stmts(d)
	var ids []int64
	var err error
	if d.Name == dialect.MySQL {
		// MySQL has no INSERT ... RETURNING; the id is the connection's own.
		if _, err = momo.Exec(ctx, tx, st.insert, e.Topic, key, payload, headers, now, now); err == nil {
			ids, err = momo.QueryScalars[int64](ctx, tx, "SELECT LAST_INSERT_ID()")
		}
	} else {
		ids, err = momo.QueryScalars[int64](ctx, tx, st.insert, e.Topic, key, payload, headers, now, now)
	}
	if err != nil {
		return 0, err
	}
	if len(ids) != 1 {
		return 0, &momo.Error{Kind: momo.ErrNotFound, Op: "outbox add", Err: errors.New("the insert returned no id")}
	}
	if d.Name == dialect.Postgres && o.notify {
		if _, err := momo.Exec(ctx, tx, "SELECT pg_notify(?, ?)", o.table, ""); err != nil {
			return 0, err
		}
	}
	return ids[0], nil
}

// cleanupChunk is the number of rows one DELETE of Cleanup removes.
const cleanupChunk = 1000

// Cleanup deletes the events that were published longer than retention ago and
// returns how many it removed. It deletes in chunks of 1000 rows, so a large
// backlog does not hold one long lock. Pending and dead events are never
// deleted: a dead event waits for an operator, who can Requeue it or delete the
// row. Run Cleanup from a scheduled job, or give a relay WithRetention to do it
// while it runs. A negative retention fails with momo.ErrValidation.
func (o *Outbox) Cleanup(ctx context.Context, q momo.Querier, retention time.Duration) (int64, error) {
	if q == nil {
		return 0, validation("outbox cleanup", "Cleanup needs a *momo.DB or *momo.Tx")
	}
	if retention < 0 {
		return 0, validation("outbox cleanup", "retention must not be negative")
	}
	d := q.Dialect()
	if err := supported(d, "outbox cleanup"); err != nil {
		return 0, err
	}
	cutoff := stamp(d, time.Now().Add(-retention))
	var total int64
	for {
		n, err := momo.Exec(ctx, q, o.stmts(d).cleanup, cutoff, cleanupChunk)
		if err != nil {
			return total, err
		}
		total += n
		if n < cleanupChunk {
			return total, nil
		}
		if err := ctx.Err(); err != nil {
			return total, err
		}
	}
}

// requeueChunk is the number of ids one UPDATE of Requeue names.
const requeueChunk = 500

// Requeue makes the dead events with the given ids pending again, with their
// attempts reset, and returns how many it changed. An id that is not dead is
// left alone. A dead event holds back the later events of its key, so a
// requeue also releases them.
func (o *Outbox) Requeue(ctx context.Context, q momo.Querier, ids ...int64) (int64, error) {
	if q == nil {
		return 0, validation("outbox requeue", "Requeue needs a *momo.DB or *momo.Tx")
	}
	d := q.Dialect()
	if err := supported(d, "outbox requeue"); err != nil {
		return 0, err
	}
	var total int64
	for start := 0; start < len(ids); start += requeueChunk {
		chunk := ids[start:min(start+requeueChunk, len(ids))]
		args := make([]any, 0, len(chunk)+1)
		args = append(args, stamp(d, time.Now()))
		for _, id := range chunk {
			args = append(args, id)
		}
		n, err := momo.Exec(ctx, q, o.stmts(d).requeue+"("+strings.Repeat("?,", len(chunk)-1)+"?)", args...)
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

// validation builds an ErrValidation error.
func validation(op, msg string) error {
	return &momo.Error{Kind: momo.ErrValidation, Op: op, Err: errors.New(msg)}
}

// stmts returns the statements for d. The caller has checked supported(d).
func (o *Outbox) stmts(d *dialect.Dialect) *statements { return o.sql[d.Name] }

// supported reports whether the outbox has statements for d.
func supported(d *dialect.Dialect, op string) error {
	switch d.Name {
	case dialect.Postgres, dialect.SQLite, dialect.MySQL:
		return nil
	}
	return &momo.Error{Kind: momo.ErrUnsupported, Op: op, Err: fmt.Errorf("the outbox runs on PostgreSQL, SQLite, MySQL and MariaDB, not %s", d.Name)}
}

// timeLayout is the fixed-width text a time is bound as on SQLite, MySQL and
// MariaDB, in UTC. On SQLite it keeps the text order of two stamps equal to
// their time order; on MySQL a datetime column holds a wall clock, and binding
// text keeps the connection's time zone out of it.
const timeLayout = "2006-01-02 15:04:05.000000"

// stamp returns t as the argument that binds to a time column of d: a UTC
// time.Time for PostgreSQL and fixed-width UTC text for the others.
func stamp(d *dialect.Dialect, t time.Time) any {
	if d.Name == dialect.Postgres {
		return t.UTC()
	}
	return t.UTC().Format(timeLayout)
}
