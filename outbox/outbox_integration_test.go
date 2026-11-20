// SPDX-License-Identifier: Apache-2.0

//go:build integration

package outbox_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	mmysql "github.com/azrtydxb/momo/migrate/mysql"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	msqlite "github.com/azrtydxb/momo/migrate/sqlite"
	"github.com/azrtydxb/momo/obs"
	"github.com/azrtydxb/momo/outbox"
)

// createTable creates the outbox table from the schema the declared model
// produces, the path `momo migrate` takes, so the tests run on the table a
// user gets.
func createTable(t *testing.T, db *momo.DB, d dialect.Name) {
	t.Helper()
	var md migrate.Dialect
	switch d {
	case dialect.Postgres:
		md = mpg.New()
	case dialect.SQLite:
		md = msqlite.New()
	case dialect.MySQL:
		md = mmysql.New()
	default:
		t.Fatalf("no migration dialect for %s", d)
	}
	e := migratetest.NewEngine(t, md, db.Driver(), migratetest.EmptyFS())
	rep, err := e.AutoMigrate(context.Background(), loadDeclared(t, d))
	if err != nil {
		t.Fatalf("create the outbox table: %v", err)
	}
	if len(rep.Skipped) != 0 {
		t.Fatalf("AutoMigrate skipped changes: %+v", rep.Skipped)
	}
}

func setup(t *testing.T, db *momo.DB, d dialect.Name, opts ...outbox.Option) *outbox.Outbox {
	t.Helper()
	createTable(t, db, d)
	o, err := outbox.New(opts...)
	if err != nil {
		t.Fatal(err)
	}
	return o
}

// collector is a Publisher that records every event it gets and fails the ones
// its fail function names.
type collector struct {
	mu   sync.Mutex
	got  []outbox.Event
	fail func(outbox.Event) error
}

func (c *collector) Publish(_ context.Context, e outbox.Event) error {
	c.mu.Lock()
	c.got = append(c.got, e)
	fail := c.fail
	c.mu.Unlock()
	if fail != nil {
		return fail(e)
	}
	return nil
}

func (c *collector) setFail(f func(outbox.Event) error) {
	c.mu.Lock()
	c.fail = f
	c.mu.Unlock()
}

func (c *collector) events() []outbox.Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]outbox.Event(nil), c.got...)
}

func newRelay(t *testing.T, o *outbox.Outbox, db *momo.DB, p outbox.Publisher, opts ...outbox.RelayOption) *outbox.Relay {
	t.Helper()
	r, err := o.NewRelay(db, p, append([]outbox.RelayOption{outbox.WithBackoff(time.Millisecond, 5*time.Millisecond)}, opts...)...)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// addAll writes events in one transaction and returns their ids.
func addAll(t *testing.T, db *momo.DB, o *outbox.Outbox, evs ...outbox.Event) []int64 {
	t.Helper()
	ctx := context.Background()
	var ids []int64
	err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
		for _, e := range evs {
			id, err := o.Add(ctx, tx, e)
			if err != nil {
				return err
			}
			ids = append(ids, id)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return ids
}

// drain runs batches until one claims nothing and returns how many events it
// claimed in all.
func drain(t *testing.T, r *outbox.Relay) int {
	t.Helper()
	total := 0
	for range 50 {
		n, err := r.RunOnce(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			return total
		}
		total += n
	}
	t.Fatal("the relay kept claiming events")
	return total
}

// state is id, attempts, published, dead and last_error of one row.
type state = momo.Row5[int64, int, bool, bool, *string]

func states(t *testing.T, db *momo.DB) []state {
	t.Helper()
	rows, err := momo.Query[state](context.Background(), db,
		`SELECT id, attempts, published_at IS NOT NULL, dead_at IS NOT NULL, last_error FROM outbox_events ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func count(t *testing.T, db *momo.DB) int64 {
	t.Helper()
	got, err := momo.QueryScalars[int64](context.Background(), db, `SELECT count(*) FROM outbox_events`)
	if err != nil || len(got) != 1 {
		t.Fatalf("count: %v %v", got, err)
	}
	return got[0]
}

// Catches: events handed out in the wrong order for one key, lost headers or
// payloads, or a published row that is not marked.
func TestRelayPublishesInKeyOrder(t *testing.T) {
	testopen.EachWithMySQL(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		o := setup(t, db, d)
		ids := addAll(t, db, o,
			outbox.Event{Topic: "orders", Key: "a", Payload: []byte("a1"), Headers: map[string]string{"trace": "t1"}},
			outbox.Event{Topic: "orders", Key: "b", Payload: []byte("b1")},
			outbox.Event{Topic: "orders", Key: "a", Payload: []byte("a2")},
			outbox.Event{Topic: "orders", Key: "a", Payload: []byte("a3")},
			outbox.Event{Topic: "audit", Payload: []byte("n1")},
			outbox.Event{Topic: "audit"},
		)
		pub := &collector{}
		r := newRelay(t, o, db, pub)
		if n := drain(t, r); n != 6 {
			t.Fatalf("claimed %d events, want 6", n)
		}
		got := pub.events()
		var a []string
		for _, e := range got {
			if e.Key == "a" {
				a = append(a, string(e.Payload))
			}
			if e.Attempt != 1 {
				t.Errorf("event %d: Attempt = %d, want 1", e.ID, e.Attempt)
			}
		}
		if strings.Join(a, ",") != "a1,a2,a3" {
			t.Errorf("key a published as %v, want a1,a2,a3", a)
		}
		byID := map[int64]outbox.Event{}
		for _, e := range got {
			byID[e.ID] = e
		}
		first := byID[ids[0]]
		if first.Topic != "orders" || first.Key != "a" || first.Headers["trace"] != "t1" {
			t.Errorf("first event = %+v", first)
		}
		if e := byID[ids[4]]; e.Key != "" || string(e.Payload) != "n1" || len(e.Headers) != 0 {
			t.Errorf("unkeyed event = %+v", e)
		}
		if e := byID[ids[5]]; len(e.Payload) != 0 {
			t.Errorf("empty payload came back as %q", e.Payload)
		}
		for _, s := range states(t, db) {
			if !s.C3 || s.C4 || s.C2 != 1 {
				t.Errorf("row %d: attempts=%d published=%v dead=%v, want 1 true false", s.C1, s.C2, s.C3, s.C4)
			}
		}
		if n := drain(t, r); n != 0 {
			t.Errorf("a second drain claimed %d events", n)
		}
	})
}

// Catches: an event that survives the rollback of the transaction that wrote
// it, which is the whole point of the outbox.
func TestAddRollsBackWithTransaction(t *testing.T) {
	testopen.EachWithMySQL(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		o := setup(t, db, d)
		abort := errors.New("abort")
		err := db.Tx(context.Background(), momo.TxOptions{}, func(tx *momo.Tx) error {
			if _, err := o.Add(context.Background(), tx, outbox.Event{Topic: "t", Payload: []byte("x")}); err != nil {
				return err
			}
			return abort
		})
		if !errors.Is(err, abort) {
			t.Fatalf("Tx err = %v, want the abort", err)
		}
		if n := count(t, db); n != 0 {
			t.Fatalf("%d rows after a rollback, want 0", n)
		}
		pub := &collector{}
		if n := drain(t, newRelay(t, o, db, pub)); n != 0 || len(pub.events()) != 0 {
			t.Fatalf("relay claimed %d events after a rollback", n)
		}
	})
}

// Catches: a retry that ignores the attempt limit, a dead event that lets a
// later event of its key overtake it, and a requeue that does not unblock the
// key.
func TestFailureRetriesThenDeadThenRequeue(t *testing.T) {
	testopen.EachWithMySQL(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		ctx := context.Background()
		o := setup(t, db, d)
		ids := addAll(t, db, o,
			outbox.Event{Topic: "t", Key: "k", Payload: []byte("1")},
			outbox.Event{Topic: "t", Key: "k", Payload: []byte("2")},
		)
		pub := &collector{}
		pub.setFail(func(outbox.Event) error { return errors.New("boom") })
		var mu sync.Mutex
		var outcomes []outbox.Outcome
		r := newRelay(t, o, db, pub,
			outbox.WithMaxAttempts(3),
			outbox.WithObserver(func(oc outbox.Outcome) {
				mu.Lock()
				outcomes = append(outcomes, oc)
				mu.Unlock()
			}))
		for range 200 {
			if _, err := r.RunOnce(ctx); err != nil {
				t.Fatal(err)
			}
			if states(t, db)[0].C4 {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		got := pub.events()
		if len(got) != 3 {
			t.Fatalf("publisher saw %d events, want 3 attempts of the first: %+v", len(got), got)
		}
		for i, e := range got {
			if e.ID != ids[0] || e.Attempt != i+1 {
				t.Errorf("call %d: id=%d attempt=%d, want id=%d attempt=%d", i, e.ID, e.Attempt, ids[0], i+1)
			}
		}
		st := states(t, db)
		if !st[0].C4 || st[0].C3 || st[0].C2 != 3 || st[0].C5 == nil || !strings.Contains(*st[0].C5, "boom") {
			t.Errorf("first row = %+v, want dead after 3 attempts with the error kept", st[0])
		}
		if st[1].C2 != 0 || st[1].C3 || st[1].C4 {
			t.Errorf("second row = %+v, want untouched behind the dead first event", st[1])
		}
		mu.Lock()
		deadFlags := make([]bool, len(outcomes))
		for i, oc := range outcomes {
			deadFlags[i] = oc.Dead
			if oc.Err == nil || oc.Event.ID != ids[0] {
				t.Errorf("outcome %d = %+v, want a failure of the first event", i, oc)
			}
		}
		mu.Unlock()
		if len(deadFlags) != 3 || deadFlags[0] || deadFlags[1] || !deadFlags[2] {
			t.Errorf("Dead flags = %v, want only the last attempt dead", deadFlags)
		}

		pub.setFail(nil)
		n, err := o.Requeue(ctx, db, ids[0])
		if err != nil || n != 1 {
			t.Fatalf("Requeue = %d, %v; want 1", n, err)
		}
		drain(t, r)
		got = pub.events()
		if len(got) != 5 || got[3].ID != ids[0] || got[3].Attempt != 1 || got[4].ID != ids[1] {
			t.Fatalf("after the requeue the publisher saw %+v", got[3:])
		}
		for _, s := range states(t, db) {
			if !s.C3 || s.C4 {
				t.Errorf("row %d: published=%v dead=%v, want published", s.C1, s.C3, s.C4)
			}
		}
	})
}

// Catches: a later event of a key published while an earlier one waits for its
// retry.
func TestFailedEventHoldsBackItsKey(t *testing.T) {
	testopen.EachWithMySQL(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		ctx := context.Background()
		o := setup(t, db, d)
		ids := addAll(t, db, o,
			outbox.Event{Topic: "t", Key: "k", Payload: []byte("1")},
			outbox.Event{Topic: "t", Key: "k", Payload: []byte("2")},
		)
		pub := &collector{}
		pub.setFail(func(e outbox.Event) error {
			if e.ID == ids[0] && e.Attempt == 1 {
				return errors.New("first try fails")
			}
			return nil
		})
		r := newRelay(t, o, db, pub)
		if n, err := r.RunOnce(ctx); err != nil || n != 1 {
			t.Fatalf("first batch = %d, %v; want exactly the head of the key", n, err)
		}
		if got := pub.events(); len(got) != 1 || got[0].ID != ids[0] {
			t.Fatalf("after the failure the publisher saw %+v", got)
		}
		for range 200 {
			if _, err := r.RunOnce(ctx); err != nil {
				t.Fatal(err)
			}
			if st := states(t, db); st[0].C3 && st[1].C3 {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		var seq []int64
		var attempts []int
		for _, e := range pub.events() {
			seq = append(seq, e.ID)
			attempts = append(attempts, e.Attempt)
		}
		if len(seq) != 3 || seq[0] != ids[0] || seq[1] != ids[0] || seq[2] != ids[1] || attempts[1] != 2 || attempts[2] != 1 {
			t.Fatalf("delivery order = %v attempts %v, want first, first again, then second", seq, attempts)
		}
	})
}

// Catches: retention that removes dead or pending rows, or that ignores the
// age.
func TestCleanupRemovesOnlyOldPublished(t *testing.T) {
	testopen.EachWithMySQL(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		ctx := context.Background()
		o := setup(t, db, d)
		addAll(t, db, o,
			outbox.Event{Topic: "t", Payload: []byte("ok1")},
			outbox.Event{Topic: "t", Payload: []byte("ok2")},
			outbox.Event{Topic: "t", Payload: []byte("bad")},
		)
		pub := &collector{}
		pub.setFail(func(e outbox.Event) error {
			if string(e.Payload) == "bad" {
				return errors.New("rejected")
			}
			return nil
		})
		r := newRelay(t, o, db, pub, outbox.WithMaxAttempts(1))
		if n := drain(t, r); n != 3 {
			t.Fatalf("claimed %d, want 3", n)
		}
		addAll(t, db, o, outbox.Event{Topic: "t", Payload: []byte("late")})
		time.Sleep(30 * time.Millisecond)

		if n, err := o.Cleanup(ctx, db, time.Hour); err != nil || n != 0 {
			t.Fatalf("Cleanup(1h) = %d, %v; want 0, nothing is that old", n, err)
		}
		n, err := o.Cleanup(ctx, db, 10*time.Millisecond)
		if err != nil || n != 2 {
			t.Fatalf("Cleanup(10ms) = %d, %v; want the 2 published rows", n, err)
		}
		st := states(t, db)
		if len(st) != 2 || !st[0].C4 || st[1].C3 || st[1].C4 {
			t.Fatalf("remaining rows = %+v, want the dead one and the pending one", st)
		}
	})
}

// Catches: the relay and Add running statements that the observability hooks
// never see.
func TestRelayStatementsReachHooks(t *testing.T) {
	rec := &obs.Recorder{}
	testopen.EachWithMySQL(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		o := setup(t, db, d)
		rec.Reset()
		addAll(t, db, o, outbox.Event{Topic: "t", Payload: []byte("x")})
		if n := drain(t, newRelay(t, o, db, &collector{})); n != 1 {
			t.Fatalf("claimed %d, want 1", n)
		}
		all := strings.Join(rec.SQLs(), "\n")
		for _, want := range []string{"INSERT INTO", "UPDATE"} {
			if !strings.Contains(all, want) {
				t.Errorf("hooks saw no %q statement:\n%s", want, all)
			}
		}
		if d != dialect.SQLite && !strings.Contains(all, "SKIP LOCKED") {
			t.Errorf("the %s claim does not use SKIP LOCKED:\n%s", d, all)
		}
	}, momo.WithHook(rec))
}
