// SPDX-License-Identifier: Apache-2.0

//go:build integration

package outbox_test

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/testdb"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/outbox"
	"github.com/jackc/pgx/v5"
)

const relayWait = 30 * time.Second

// Catches: a relay that does not stop on cancel, that counts the cancelled
// publish as a failed attempt, or that leaves the event leased so another
// relay must wait for the lease to run out.
func TestRunStopsOnCancelAndReleasesEvent(t *testing.T) {
	testopen.EachWithMySQL(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		o := setup(t, db, d)
		addAll(t, db, o, outbox.Event{Topic: "t", Payload: []byte("x")})

		entered := make(chan struct{}, 1)
		blocked := outbox.PublisherFunc(func(ctx context.Context, _ outbox.Event) error {
			select {
			case entered <- struct{}{}:
			default:
			}
			<-ctx.Done()
			return ctx.Err()
		})
		r := newRelay(t, o, db, blocked, outbox.WithPollInterval(5*time.Millisecond), outbox.WithLease(time.Hour))
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		done := make(chan error, 1)
		go func() { done <- r.Run(ctx) }()

		select {
		case <-entered:
		case <-time.After(relayWait):
			t.Fatal("the relay never published")
		}
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("Run returned %v after cancel, want nil", err)
			}
		case <-time.After(relayWait):
			t.Fatal("Run did not return after cancel")
		}

		st := states(t, db)
		if len(st) != 1 || st[0].C2 != 0 || st[0].C3 || st[0].C4 {
			t.Fatalf("row after cancel = %+v, want released: attempts 0, pending", st)
		}
		pub := &collector{}
		if n, err := newRelay(t, o, db, pub).RunOnce(context.Background()); err != nil || n != 1 {
			t.Fatalf("a new relay claimed %d, %v; want the released event at once", n, err)
		}
		if got := pub.events(); len(got) != 1 || got[0].Attempt != 1 {
			t.Fatalf("the new relay saw %+v, want attempt 1", got)
		}
	})
}

// Catches: two relays that publish the same event, which is what SKIP LOCKED
// and the lease are there to prevent.
func TestTwoRelaysPublishEachEventOnce(t *testing.T) {
	testopen.EachWithMySQL(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		if d == dialect.SQLite {
			t.Skip("SQLite has one writer; relays on one file are not run concurrently here")
		}
		twoRelays(t, db, d)
	})
}

func twoRelays(t *testing.T, db *momo.DB, d dialect.Name) {
	t.Helper()
	o := setup(t, db, d)
	const total = 60
	evs := make([]outbox.Event, total)
	for i := range evs {
		evs[i] = outbox.Event{Topic: "t", Payload: []byte(strconv.Itoa(i))}
	}
	addAll(t, db, o, evs...)

	var mu sync.Mutex
	seen := map[int64]int{}
	pub := outbox.PublisherFunc(func(_ context.Context, e outbox.Event) error {
		mu.Lock()
		seen[e.ID]++
		mu.Unlock()
		return nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*relayWait)
	defer cancel()
	var wg sync.WaitGroup
	for range 2 {
		r := newRelay(t, o, db, pub, outbox.WithBatchSize(5), outbox.WithPollInterval(5*time.Millisecond))
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = r.Run(ctx)
		}()
	}
	for {
		mu.Lock()
		n := len(seen)
		mu.Unlock()
		if n == total {
			break
		}
		if ctx.Err() != nil {
			t.Fatalf("only %d of %d events were published", n, total)
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	wg.Wait()
	for id, c := range seen {
		if c != 1 {
			t.Errorf("event %d was published %d times, want 1", id, c)
		}
	}
	for _, s := range states(t, db) {
		if !s.C3 {
			t.Errorf("row %d is not marked published", s.C1)
		}
	}
}

// Catches: two relays that publish the events of one key out of order, which
// the per-key rule in the claim exists to prevent.
func TestTwoRelaysKeepKeyOrder(t *testing.T) {
	testopen.EachWithMySQL(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		if d == dialect.SQLite {
			t.Skip("SQLite has one writer; relays on one file are not run concurrently here")
		}
		o := setup(t, db, d)
		const keys, perKey = 5, 6
		var evs []outbox.Event
		for n := range perKey {
			for k := range keys {
				evs = append(evs, outbox.Event{Topic: "t", Key: "k" + strconv.Itoa(k), Payload: []byte(strconv.Itoa(n))})
			}
		}
		addAll(t, db, o, evs...)

		var mu sync.Mutex
		got := map[string][]int{}
		total := 0
		pub := outbox.PublisherFunc(func(_ context.Context, e outbox.Event) error {
			n, err := strconv.Atoi(string(e.Payload))
			if err != nil {
				return err
			}
			mu.Lock()
			got[e.Key] = append(got[e.Key], n)
			total++
			mu.Unlock()
			return nil
		})
		ctx, cancel := context.WithTimeout(context.Background(), 2*relayWait)
		defer cancel()
		var wg sync.WaitGroup
		for range 2 {
			r := newRelay(t, o, db, pub, outbox.WithBatchSize(3), outbox.WithPollInterval(5*time.Millisecond))
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = r.Run(ctx)
			}()
		}
		for {
			mu.Lock()
			n := total
			mu.Unlock()
			if n >= keys*perKey {
				break
			}
			if ctx.Err() != nil {
				t.Fatalf("only %d of %d events were published", n, keys*perKey)
			}
			time.Sleep(10 * time.Millisecond)
		}
		cancel()
		wg.Wait()
		mu.Lock()
		defer mu.Unlock()
		for key, seq := range got {
			for i, n := range seq {
				if n != i {
					t.Errorf("key %s was published as %v, want 0 to %d in order", key, seq, perKey-1)
					break
				}
			}
		}
	})
}

// listenerCount counts the sessions of the test database that last ran LISTEN
// and are idle: the relays' listeners waiting for a notification.
func listenerCount(t *testing.T, db *momo.DB) int64 {
	t.Helper()
	got, err := momo.Scalars[int64](context.Background(), db,
		`SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND state = 'idle' AND query LIKE 'LISTEN%'`)
	if err != nil || len(got) != 1 {
		t.Fatalf("listener count: %v %v", got, err)
	}
	return got[0]
}

func startListeningRelay(t *testing.T, db *momo.DB, o *outbox.Outbox, p outbox.Publisher) *outbox.Relay {
	t.Helper()
	cfg, err := pgx.ParseConfig(testdb.PostgresURL(t))
	if err != nil {
		t.Fatal(err)
	}
	// A poll interval of an hour leaves the notification, or Wake, as the only
	// thing that can make the relay look again.
	r := newRelay(t, o, db, p, outbox.WithPollInterval(time.Hour), outbox.WithListen(cfg))
	before := listenerCount(t, db)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = r.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(relayWait):
			t.Error("Run did not return after cancel")
		}
	})
	deadline := time.Now().Add(relayWait)
	for listenerCount(t, db) <= before {
		if time.Now().After(deadline) {
			t.Fatal("the relay never started listening")
		}
		time.Sleep(10 * time.Millisecond)
	}
	// Let the relay finish its first, empty pass and go idle.
	time.Sleep(500 * time.Millisecond)
	return r
}

// Catches: a relay that only polls, so an event waits up to a poll interval
// after its transaction committed.
func TestNotifyWakesRelay(t *testing.T) {
	db := testopen.Postgres(t)
	o := setup(t, db, dialect.Postgres)
	got := make(chan outbox.Event, 4)
	startListeningRelay(t, db, o, outbox.PublisherFunc(func(_ context.Context, e outbox.Event) error {
		got <- e
		return nil
	}))
	addAll(t, db, o, outbox.Event{Topic: "t", Payload: []byte("wake")})
	select {
	case e := <-got:
		if string(e.Payload) != "wake" {
			t.Fatalf("published %+v", e)
		}
	case <-time.After(relayWait):
		t.Fatal("the relay was not woken by the notification")
	}
}

// Catches: WithoutNotify still notifying, and Wake not waking an idle relay.
func TestWithoutNotifyNeedsWake(t *testing.T) {
	db := testopen.Postgres(t)
	o := setup(t, db, dialect.Postgres, outbox.WithoutNotify())
	got := make(chan outbox.Event, 4)
	r := startListeningRelay(t, db, o, outbox.PublisherFunc(func(_ context.Context, e outbox.Event) error {
		got <- e
		return nil
	}))
	addAll(t, db, o, outbox.Event{Topic: "t", Payload: []byte("quiet")})
	select {
	case e := <-got:
		t.Fatalf("published %+v without a notification or a wake", e)
	case <-time.After(750 * time.Millisecond):
	}
	r.Wake()
	select {
	case <-got:
	case <-time.After(relayWait):
		t.Fatal("Wake did not make the relay look again")
	}
}
