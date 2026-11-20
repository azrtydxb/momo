// SPDX-License-Identifier: Apache-2.0

package outbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
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

// ErrLeaseLost is reported to the WithOnError function when the relay could not
// record the outcome of an event because another relay holds it now: the lease
// ended before the publish returned, and the event was claimed again, so the
// statement that records the outcome matched no row. The outcome is dropped,
// and the relay that holds the event decides what happens to it. The event may
// be published twice. The reported error wraps ErrLeaseLost.
var ErrLeaseLost = errors.New("outbox: the lease on the event ended before its outcome was recorded")

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
	// LeaseLost reports that the lease on the event ended and another relay
	// claimed it before this outcome could be recorded, so the outcome was
	// dropped and Dead is false; see ErrLeaseLost. Count it to see publishes
	// that outlive WithLease.
	LeaseLost bool
	// Duration is how long Publish ran.
	Duration time.Duration
}

type relayConfig struct {
	rand        func() float64
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
// is one second up to five minutes. Each delay is then spread at random by up
// to 20 percent either way, so events that failed together do not come back
// together, and never exceeds top.
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
// itself: a failed claim, a failed bookkeeping statement, an outcome dropped
// because the lease was lost (an error that wraps ErrLeaseLost), a header value
// that is not a JSON object of strings, a publisher panic (with its stack), an
// event abandoned because it exhausted its attempts while a relay was publishing
// it, a failed LISTEN. Run keeps going after each. The default logs a warning
// with log/slog.
func WithOnError(fn func(error)) RelayOption { return func(c *relayConfig) { c.onError = fn } }

// Relay claims the events of an outbox and hands them to a Publisher. Create it
// with Outbox.NewRelay. It is safe for concurrent use, though one Run per Relay
// is the intended use; run several Relays, in one process or many, to share the
// work.
type Relay struct {
	o        *Outbox
	db       *momo.DB
	d        *dialect.Dialect
	st       *statements
	pub      Publisher
	cfg      relayConfig
	wake     chan struct{}
	serverOK atomic.Bool
}

// NewRelay returns a Relay that publishes the events of o, read through db,
// with pub. It fails with momo.ErrValidation for a nil argument or an option
// out of range, and with momo.ErrUnsupported for WithListen on a database other
// than PostgreSQL. A MySQL or MariaDB server too old for SKIP LOCKED (before
// MySQL 8.0 or MariaDB 10.6) is found when the relay first claims: RunOnce
// returns momo.ErrUnsupported, and so does Run, which stops.
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
		rand:        rand.Float64,
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
	return &Relay{o: o, db: db, d: d, st: o.stmts(d), pub: pub, cfg: cfg, wake: make(chan struct{}, 1)}, nil
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
		if errors.Is(err, momo.ErrUnsupported) {
			return err
		}
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
//
// A claim counts an attempt before the event is published. A relay that crashes
// while it publishes therefore loses that attempt, and an event that keeps
// crashing its relay reaches the maximum of attempts: the next claim then marks
// it dead without publishing it, so a poison event does not take the process
// down forever.
func (r *Relay) RunOnce(ctx context.Context) (int, error) {
	if err := r.checkServer(ctx); err != nil {
		return 0, err
	}
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
		if ev.Attempt > r.cfg.maxAttempts {
			r.abandon(ctx, ev)
			continue
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
	switch r.d.Name {
	case dialect.SQLite:
		rows, err = momo.Query[claimRow](ctx, r.db, r.st.claim, stamp(r.d, deadline), stamp(r.d, now), r.cfg.batch)
	case dialect.MySQL:
		rows, err = r.claimMySQL(ctx, now, deadline)
	default:
		rows, err = momo.Query[claimRow](ctx, r.db, r.st.claim, stamp(r.d, now), r.cfg.batch, stamp(r.d, deadline))
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
			// Add; the event goes out without headers rather than not at all,
			// and the error is reported.
			if err := json.Unmarshal([]byte(*row.C5), &ev.Headers); err != nil {
				ev.Headers = nil
				r.cfg.onError(fmt.Errorf("outbox: event %d has headers that are not a JSON object of strings, publishing it without: %w", ev.ID, err))
			}
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

// claimMySQL claims a batch on MySQL and MariaDB in one short transaction: it
// selects the due rows with FOR UPDATE SKIP LOCKED, leases them and commits.
// READ COMMITTED avoids the gap locks of the default isolation level. Nothing
// is published inside the transaction.
func (r *Relay) claimMySQL(ctx context.Context, now, deadline time.Time) ([]claimRow, error) {
	var rows []claimRow
	err := r.db.Tx(ctx, momo.TxOptions{Isolation: sql.LevelReadCommitted}, func(tx *momo.Tx) error {
		var err error
		rows, err = momo.Query[claimRow](ctx, tx, r.st.claim, stamp(r.d, now), r.cfg.batch)
		if err != nil || len(rows) == 0 {
			return err
		}
		args := make([]any, 0, len(rows)+1)
		args = append(args, stamp(r.d, deadline))
		for _, row := range rows {
			args = append(args, row.C1)
		}
		_, err = momo.Exec(ctx, tx, r.st.lease+"("+strings.Repeat("?,", len(rows)-1)+"?)", args...)
		return err
	})
	if err != nil {
		return nil, err
	}
	// The select ran before the lease counted the attempt; PostgreSQL and
	// SQLite return the row after their UPDATE, so match them.
	for i := range rows {
		rows[i].C6++
	}
	return rows, nil
}

// checkServer fails with momo.ErrUnsupported when a MySQL or MariaDB server is
// too old for SKIP LOCKED. It asks the server once and remembers a success.
func (r *Relay) checkServer(ctx context.Context) error {
	if r.d.Name != dialect.MySQL || r.serverOK.Load() {
		return nil
	}
	got, err := momo.QueryScalars[string](ctx, r.db, "SELECT VERSION()")
	if err != nil {
		return err
	}
	if len(got) != 1 {
		return &momo.Error{Kind: momo.ErrNotFound, Op: "outbox relay", Err: errors.New("SELECT VERSION() returned no row")}
	}
	if err := skipLockedSupported(r.d.Flavor, got[0]); err != nil {
		return err
	}
	r.serverOK.Store(true)
	return nil
}

// skipLockedSupported reports whether a server of the given flavor and
// VERSION() string has FOR UPDATE SKIP LOCKED: MySQL 8.0 and MariaDB 10.6. A
// version it cannot read is refused rather than guessed.
func skipLockedSupported(flavor dialect.Flavor, version string) error {
	minMajor, minMinor, name := 8, 0, "MySQL 8.0"
	if flavor == dialect.FlavorMariaDB {
		minMajor, minMinor, name = 10, 6, "MariaDB 10.6"
	}
	v := strings.TrimPrefix(version, "5.5.5-")
	parts := strings.SplitN(v, ".", 3)
	major, err := strconv.Atoi(parts[0])
	minor := -1
	if err == nil && len(parts) > 1 {
		end := 0
		for end < len(parts[1]) && parts[1][end] >= '0' && parts[1][end] <= '9' {
			end++
		}
		if m, merr := strconv.Atoi(parts[1][:end]); merr == nil {
			minor = m
		}
	}
	if err != nil || minor < 0 {
		return &momo.Error{Kind: momo.ErrUnsupported, Op: "outbox relay", Err: fmt.Errorf("the outbox needs FOR UPDATE SKIP LOCKED (%s or newer) and the server version %q cannot be read", name, version)}
	}
	if major < minMajor || (major == minMajor && minor < minMinor) {
		return &momo.Error{Kind: momo.ErrUnsupported, Op: "outbox relay", Err: fmt.Errorf("the outbox needs FOR UPDATE SKIP LOCKED (%s or newer), and the server is %s", name, version)}
	}
	return nil
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
		rerr := r.record(bg, ev, "published", r.st.published, stamp(r.d, now), ev.ID, ev.Attempt)
		r.observe(Outcome{Event: ev, Duration: took, LeaseLost: errors.Is(rerr, ErrLeaseLost)})
	case ctx.Err() != nil:
		// The relay is stopping and cut the publish short: that is not the
		// event's failure.
		r.release(ctx, []Event{ev})
	default:
		msg := truncate(err.Error())
		dead := ev.Attempt >= r.cfg.maxAttempts
		var rerr error
		if dead {
			rerr = r.record(bg, ev, "dead", r.st.dead, stamp(r.d, now), msg, ev.ID, ev.Attempt)
		} else {
			rerr = r.record(bg, ev, "retry", r.st.retry, stamp(r.d, now.Add(r.backoff(ev.Attempt))), msg, ev.ID, ev.Attempt)
		}
		lost := errors.Is(rerr, ErrLeaseLost)
		r.observe(Outcome{Event: ev, Err: err, Dead: dead && !lost, LeaseLost: lost, Duration: took})
	}
}

// abandon marks dead an event whose claim counted more attempts than the
// maximum: an earlier relay claimed it for the last allowed attempt and never
// recorded how that attempt ended, which is what a publish that crashes the
// process looks like. It is not published again.
func (r *Relay) abandon(ctx context.Context, ev Event) {
	bg, done := bookkeeping(ctx)
	defer done()
	err := fmt.Errorf("outbox: event %d was claimed for its last allowed attempt (%d) and that attempt never ended, so the relay crashed or lost its lease while publishing it; it is dead and was not published again", ev.ID, r.cfg.maxAttempts)
	rerr := r.record(bg, ev, "dead", r.st.dead, stamp(r.d, time.Now()), truncate(err.Error()), ev.ID, ev.Attempt)
	lost := errors.Is(rerr, ErrLeaseLost)
	if !lost {
		r.cfg.onError(err)
	}
	r.observe(Outcome{Event: ev, Err: err, Dead: !lost, LeaseLost: lost})
}

// publish calls the Publisher and turns a panic into an error, so a bug in one
// event's publishing is retried like any failure and does not end the relay.
// The panic and its stack go to the WithOnError function.
func (r *Relay) publish(ctx context.Context, ev Event) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("outbox: the publisher panicked: %v", p)
			r.cfg.onError(fmt.Errorf("%w\n%s", err, debug.Stack()))
		}
	}()
	return r.pub.Publish(ctx, ev)
}

// record runs one outcome statement of ev, fenced to the attempt the relay
// claimed, and reports a failure to the WithOnError function. It returns nil
// when the statement changed the row, an error that wraps ErrLeaseLost when it
// matched none, and the database error otherwise. When it fails, the event
// stays leased and is claimed again when the lease ends.
func (r *Relay) record(ctx context.Context, ev Event, what, sql string, args ...any) error {
	n, err := momo.Exec(ctx, r.db, sql, args...)
	if err == nil && n == 0 {
		err = fmt.Errorf("%w: event %d, attempt %d, %s not recorded", ErrLeaseLost, ev.ID, ev.Attempt, what)
	}
	if err != nil {
		r.cfg.onError(err)
	}
	return err
}

// release makes claimed events due again at once and gives back the attempt the
// claim counted. An event whose lease was lost is left to its new holder.
func (r *Relay) release(ctx context.Context, evs []Event) {
	bg, done := bookkeeping(ctx)
	defer done()
	now := stamp(r.d, time.Now())
	for _, ev := range evs {
		_ = r.record(bg, ev, "release", r.st.release, now, ev.ID, ev.Attempt)
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
	d = min(d, r.cfg.backoffMax)
	// Spread the delay by 20 percent either way: rand returns [0, 1).
	d = time.Duration(float64(d) * (0.8 + 0.4*r.cfg.rand()))
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
