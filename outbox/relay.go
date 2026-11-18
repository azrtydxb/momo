// SPDX-License-Identifier: Apache-2.0

package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/jackc/pgx/v5"
)

// maxBatch is the largest batch size a relay accepts.
const maxBatch = 1000

// maxErrorBytes bounds the last_error text kept on a row.
const maxErrorBytes = 1000

// cleanupEvery is how often Run applies WithRetention.
const cleanupEvery = time.Minute

// bookkeepingTimeout bounds the statements that record the outcome of a
// delivery after the relay's context has ended.
const bookkeepingTimeout = 10 * time.Second

// Outcome is the result of one delivery attempt, passed to the function given
// to WithObserver.
type Outcome struct {
	// Event is the event, with Attempt set to the attempt that just ended.
	Event Event
	// Err is nil when the event was published, and the Publisher's error when
	// it was not.
	Err error
	// Dead reports that Err was the last allowed attempt: the event is now
	// dead and waits for Requeue.
	Dead bool
	// Duration is how long Publish ran.
	Duration time.Duration
}

type relayConfig struct {
	batch       int
	lease       time.Duration
	poll        time.Duration
	backoffBase time.Duration
	backoffMax  time.Duration
	maxAttempts int
	retention   time.Duration
	observer    func(Outcome)
	onError     func(error)
	listen      *pgx.ConnConfig
	listenSet   bool
}

// RelayOption configures a Relay.
type RelayOption func(*relayConfig)

// WithBatchSize sets how many events one claim takes, from 1 to 1000. The
// default is 100. The events of one batch are published one after the other,
// so the batch has to fit in the lease.
func WithBatchSize(n int) RelayOption { return func(c *relayConfig) { c.batch = n } }

// WithLease sets how long a claimed event is reserved for the relay that
// claimed it. The default is 30 seconds. A batch is published within one lease:
// the context passed to Publish ends at the end of the lease, and the events of
// the batch that were not reached by then are released, not counted as
// attempts. Choose a lease longer than a batch takes to publish.
func WithLease(d time.Duration) RelayOption { return func(c *relayConfig) { c.lease = d } }

// WithPollInterval sets how long an idle Run waits before it looks for events
// again. A notification or Relay.Wake ends the wait early. The default is one
// second.
func WithPollInterval(d time.Duration) RelayOption { return func(c *relayConfig) { c.poll = d } }

// WithBackoff sets the delay before a failed event is tried again: base after
// the first failure, doubling with every further failure up to top. The default
// is one second up to five minutes.
func WithBackoff(base, top time.Duration) RelayOption {
	return func(c *relayConfig) { c.backoffBase, c.backoffMax = base, top }
}

// WithMaxAttempts sets how many times an event is tried before it becomes dead.
// The default is 10.
func WithMaxAttempts(n int) RelayOption { return func(c *relayConfig) { c.maxAttempts = n } }

// WithRetention makes Run delete events published longer than d ago, once a
// minute, as Outbox.Cleanup does. Zero, the default, turns it off.
func WithRetention(d time.Duration) RelayOption { return func(c *relayConfig) { c.retention = d } }

// WithObserver sets a function called after every delivery attempt with its
// Outcome, on the goroutine that ran it. Use it for metrics and logs; the
// statements the relay runs also reach the hooks of the *momo.DB. A panic in fn
// is recovered and dropped.
func WithObserver(fn func(Outcome)) RelayOption { return func(c *relayConfig) { c.observer = fn } }

// WithOnError sets a function called with the errors the relay cannot handle
// itself: a failed claim, a failed bookkeeping statement, a failed LISTEN. Run
// keeps going after each. The default logs a warning with log/slog.
func WithOnError(fn func(error)) RelayOption { return func(c *relayConfig) { c.onError = fn } }

// Relay claims the events of an outbox and hands them to a Publisher. Create it
// with Outbox.NewRelay. It is safe for concurrent use, though one Run per Relay
// is the intended use; run several Relays, in one process or many, to share the
// work.
type Relay struct {
	o    *Outbox
	db   *momo.DB
	d    *dialect.Dialect
	pub  Publisher
	cfg  relayConfig
	wake chan struct{}
}

// NewRelay returns a Relay that publishes the events of o, read through db,
// with pub. It fails with momo.ErrValidation for a nil argument or an option
// out of range, and with momo.ErrUnsupported for MySQL and MariaDB and for
// WithListen on a database other than PostgreSQL.
func (o *Outbox) NewRelay(db *momo.DB, pub Publisher, opts ...RelayOption) (*Relay, error) {
	if db == nil {
		return nil, validation("outbox relay", "NewRelay needs a *momo.DB")
	}
	if pub == nil {
		return nil, validation("outbox relay", "NewRelay needs a Publisher")
	}
	d := db.Dialect()
	if err := supported(d, "outbox relay"); err != nil {
		return nil, err
	}
	cfg := relayConfig{
		batch:       100,
		lease:       30 * time.Second,
		poll:        time.Second,
		backoffBase: time.Second,
		backoffMax:  5 * time.Minute,
		maxAttempts: 10,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	switch {
	case cfg.batch < 1 || cfg.batch > maxBatch:
		return nil, validation("outbox relay", fmt.Sprintf("the batch size must be 1 to %d", maxBatch))
	case cfg.lease <= 0:
		return nil, validation("outbox relay", "the lease must be positive")
	case cfg.poll <= 0:
		return nil, validation("outbox relay", "the poll interval must be positive")
	case cfg.backoffBase <= 0 || cfg.backoffMax < cfg.backoffBase:
		return nil, validation("outbox relay", "the backoff base must be positive and the maximum at least the base")
	case cfg.maxAttempts < 1:
		return nil, validation("outbox relay", "the maximum attempts must be at least 1")
	case cfg.retention < 0:
		return nil, validation("outbox relay", "the retention must not be negative")
	}
	if cfg.listenSet {
		if cfg.listen == nil {
			return nil, validation("outbox relay", "WithListen needs a connection config")
		}
		if d.Name != dialect.Postgres {
			return nil, &momo.Error{Kind: momo.ErrUnsupported, Op: "outbox relay", Err: fmt.Errorf("LISTEN and NOTIFY need PostgreSQL, not %s: poll instead", d.Name)}
		}
	}
	if cfg.onError == nil {
		table := o.table
		cfg.onError = func(err error) { slog.Warn("outbox relay", "table", table, "err", err) }
	}
	return &Relay{o: o, db: db, d: d, pub: pub, cfg: cfg, wake: make(chan struct{}, 1)}, nil
}

// Wake makes an idle Run look for events now instead of at the end of its poll
// interval. It never blocks and may be called from any goroutine, for example
// from the code that just committed an event.
func (r *Relay) Wake() {
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

// Run publishes events until ctx ends, and then returns nil. It claims and
// publishes batches back to back while there is work, and otherwise waits for
// the poll interval, a notification or Wake. Errors it cannot handle go to the
// WithOnError function and do not stop it.
//
// When ctx ends, the publish in progress sees its context end, and the relay
// records what it can: a publish that succeeded is marked published, and an
// event that was interrupted is released at once without counting an attempt,
// so another relay does not wait for the lease. The bookkeeping uses a context
// that outlives ctx for up to ten seconds.
func (r *Relay) Run(ctx context.Context) error {
	stop := r.startListener(ctx)
	defer stop()
	var lastCleanup time.Time
	for {
		if ctx.Err() != nil {
			return nil
		}
		n, err := r.RunOnce(ctx)
		if err != nil && ctx.Err() == nil {
			r.cfg.onError(err)
		}
		r.cleanup(ctx, &lastCleanup)
		if ctx.Err() != nil {
			return nil
		}
		if err == nil && n > 0 {
			continue
		}
		r.idle(ctx)
	}
}

// idle waits for the poll interval, a wake-up or the end of ctx.
func (r *Relay) idle(ctx context.Context) {
	t := time.NewTimer(r.cfg.poll)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-r.wake:
	case <-t.C:
	}
}

// cleanup applies the retention at most once per cleanupEvery.
func (r *Relay) cleanup(ctx context.Context, last *time.Time) {
	if r.cfg.retention <= 0 || time.Since(*last) < cleanupEvery {
		return
	}
	*last = time.Now()
	if _, err := r.o.Cleanup(ctx, r.db, r.cfg.retention); err != nil && ctx.Err() == nil {
		r.cfg.onError(err)
	}
}

// RunOnce claims one batch of due events and publishes them in id order. It
// returns how many events it claimed; zero means nothing was due. A claim error
// is returned; a failure to publish or to record an outcome is not, it goes to
// the observer and the WithOnError function.
//
// Only the first event of each key is claimed, so a key with a backlog
// advances by one event per batch. Run calls RunOnce again at once when a batch
// was not empty.
func (r *Relay) RunOnce(ctx context.Context) (int, error) {
	now := time.Now()
	deadline := now.Add(r.cfg.lease)
	batch, err := r.claim(ctx, now, deadline)
	if err != nil {
		return 0, err
	}
	for i, ev := range batch {
		if ctx.Err() != nil || !time.Now().Before(deadline) {
			r.release(ctx, batch[i:])
			break
		}
		r.deliver(ctx, ev, deadline)
	}
	return len(batch), nil
}

// claimRow is id, topic, key, payload, headers and attempts of a claimed row.
type claimRow = momo.Row6[int64, string, *string, []byte, *string, int]

// claim takes the due events and leases them until deadline.
func (r *Relay) claim(ctx context.Context, now, deadline time.Time) ([]Event, error) {
	var rows []claimRow
	var err error
	if r.d.Name == dialect.SQLite {
		rows, err = momo.Query[claimRow](ctx, r.db, r.o.sql.claimSQLite, stamp(r.d, deadline), stamp(r.d, now), r.cfg.batch)
	} else {
		rows, err = momo.Query[claimRow](ctx, r.db, r.o.sql.claimPG, stamp(r.d, now), r.cfg.batch, stamp(r.d, deadline))
	}
	if err != nil {
		return nil, err
	}
	out := make([]Event, 0, len(rows))
	for _, row := range rows {
		ev := Event{ID: row.C1, Topic: row.C2, Payload: row.C4, Attempt: row.C6}
		if row.C3 != nil {
			ev.Key = *row.C3
		}
		if row.C5 != nil && *row.C5 != "" {
			// A value that is not a JSON object of strings was not written by
			// Add; the event goes out without headers rather than not at all.
			_ = json.Unmarshal([]byte(*row.C5), &ev.Headers)
		}
		out = append(out, ev)
	}
	slices.SortFunc(out, func(a, b Event) int {
		switch {
		case a.ID < b.ID:
			return -1
		case a.ID > b.ID:
			return 1
		}
		return 0
	})
	return out, nil
}

// bookkeeping returns a context for recording an outcome: it ignores the
// cancellation of ctx, so a relay that is stopping still records what happened.
func bookkeeping(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), bookkeepingTimeout)
}

// deliver publishes one claimed event and records the outcome.
func (r *Relay) deliver(ctx context.Context, ev Event, deadline time.Time) {
	pctx, cancel := context.WithDeadline(ctx, deadline)
	start := time.Now()
	err := r.publish(pctx, ev)
	cancel()
	took := time.Since(start)

	bg, done := bookkeeping(ctx)
	defer done()
	now := time.Now()
	switch {
	case err == nil:
		r.record(bg, r.o.sql.published, stamp(r.d, now), ev.ID)
		r.observe(Outcome{Event: ev, Duration: took})
	case ctx.Err() != nil:
		// The relay is stopping and cut the publish short: that is not the
		// event's failure.
		r.release(ctx, []Event{ev})
	default:
		msg := truncate(err.Error())
		dead := ev.Attempt >= r.cfg.maxAttempts
		if dead {
			r.record(bg, r.o.sql.dead, stamp(r.d, now), msg, ev.ID)
		} else {
			r.record(bg, r.o.sql.retry, stamp(r.d, now.Add(r.backoff(ev.Attempt))), msg, ev.ID)
		}
		r.observe(Outcome{Event: ev, Err: err, Dead: dead, Duration: took})
	}
}

// publish calls the Publisher and turns a panic into an error, so a bug in one
// event's publishing is retried like any failure and does not end the relay.
func (r *Relay) publish(ctx context.Context, ev Event) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("outbox: the publisher panicked: %v", p)
		}
	}()
	return r.pub.Publish(ctx, ev)
}

// record runs one bookkeeping statement and reports its error. The event stays
// leased when it fails, and is claimed again when the lease ends.
func (r *Relay) record(ctx context.Context, sql string, args ...any) {
	if _, err := momo.Exec(ctx, r.db, sql, args...); err != nil {
		r.cfg.onError(err)
	}
}

// release makes claimed events due again at once and gives back the attempt the
// claim counted.
func (r *Relay) release(ctx context.Context, evs []Event) {
	bg, done := bookkeeping(ctx)
	defer done()
	now := stamp(r.d, time.Now())
	for _, ev := range evs {
		r.record(bg, r.o.sql.release, now, ev.ID)
	}
}

// observe calls the observer, if any, and survives its panic.
func (r *Relay) observe(oc Outcome) {
	if r.cfg.observer == nil {
		return
	}
	defer func() { _ = recover() }()
	r.cfg.observer(oc)
}

// backoff returns the delay after the attempt-th failed attempt.
func (r *Relay) backoff(attempt int) time.Duration {
	d := r.cfg.backoffBase
	for i := 1; i < attempt && d < r.cfg.backoffMax; i++ {
		d *= 2
	}
	return min(d, r.cfg.backoffMax)
}

// truncate shortens s to maxErrorBytes without cutting a character.
func truncate(s string) string {
	if len(s) <= maxErrorBytes {
		return s
	}
	cut := maxErrorBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return strings.ToValidUTF8(s[:cut], "")
}
