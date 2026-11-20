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
	"github.com/azrtydxb/momo/outbox"
)

// at is t as the argument that binds to a time column of d.
func at(d dialect.Name, t time.Time) any {
	if d == dialect.Postgres {
		return t.UTC()
	}
	return t.UTC().Format("2006-01-02 15:04:05.000000")
}

// claimByHand does what a relay's claim does and then never finishes: it
// counts attempts as the attempt number and leases the event until leaseEnd.
func claimByHand(t *testing.T, db *momo.DB, d dialect.Name, id int64, attempts int, leaseEnd time.Time) {
	t.Helper()
	if _, err := momo.Exec(context.Background(), db,
		`UPDATE outbox_events SET attempts = ?, available_at = ? WHERE id = ?`, attempts, at(d, leaseEnd), id); err != nil {
		t.Fatal(err)
	}
}

// errorLog is a WithOnError function that keeps what it is given.
type errorLog struct {
	mu   sync.Mutex
	errs []error
}

func (l *errorLog) add(err error) {
	l.mu.Lock()
	l.errs = append(l.errs, err)
	l.mu.Unlock()
}

func (l *errorLog) all() []error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]error(nil), l.errs...)
}

// waitFor receives from ch or fails the test after relayWait.
func waitFor(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(relayWait):
		t.Fatalf("timed out waiting for %s", what)
	}
}

// Catches: a relay whose lease ended writing its outcome over the relay that
// holds the event now. The first relay's publish outlives its lease and fails
// late; a second relay has claimed the event and is publishing it. The late
// failure must not touch the row: no retry time, no error text, no change to
// the attempt count the new holder counted.
func TestStaleRelayOutcomeIsIgnored(t *testing.T) {
	testopen.EachWithMySQL(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		o := setup(t, db, d)
		addAll(t, db, o, outbox.Event{Topic: "t", Payload: []byte("x")})

		aStarted := make(chan struct{})
		releaseA := make(chan struct{})
		aDone := make(chan error, 1)
		var mu sync.Mutex
		var reported []error
		var outcomes []outbox.Outcome
		stale := newRelay(t, o, db, outbox.PublisherFunc(func(context.Context, outbox.Event) error {
			close(aStarted)
			<-releaseA
			return errors.New("late failure")
		}), outbox.WithLease(200*time.Millisecond),
			outbox.WithOnError(func(err error) {
				mu.Lock()
				reported = append(reported, err)
				mu.Unlock()
			}),
			outbox.WithObserver(func(oc outbox.Outcome) {
				mu.Lock()
				outcomes = append(outcomes, oc)
				mu.Unlock()
			}))
		go func() {
			_, err := stale.RunOnce(context.Background())
			aDone <- err
		}()
		waitFor(t, aStarted, "the first relay to publish")
		time.Sleep(400 * time.Millisecond) // the lease of the first relay ends

		bStarted := make(chan struct{})
		releaseB := make(chan struct{})
		bDone := make(chan error, 1)
		holder := newRelay(t, o, db, outbox.PublisherFunc(func(_ context.Context, e outbox.Event) error {
			if e.Attempt != 2 {
				t.Errorf("the second relay saw attempt %d, want 2", e.Attempt)
			}
			close(bStarted)
			<-releaseB
			return nil
		}))
		go func() {
			n, err := holder.RunOnce(context.Background())
			if err == nil && n != 1 {
				err = errors.New("the second relay did not claim the event after the lease ended")
			}
			bDone <- err
		}()
		waitFor(t, bStarted, "the second relay to publish")

		close(releaseA)
		if err := <-aDone; err != nil {
			t.Fatalf("the first relay: %v", err)
		}
		st := states(t, db)
		if len(st) != 1 || st[0].C2 != 2 || st[0].C3 || st[0].C4 || st[0].C5 != nil {
			t.Fatalf("row after the stale outcome = %+v, want attempt 2, pending, no error: the late failure was written", st)
		}

		mu.Lock()
		if len(reported) != 1 || !errors.Is(reported[0], outbox.ErrLeaseLost) {
			t.Errorf("WithOnError got %v, want one error wrapping ErrLeaseLost", reported)
		}
		if len(outcomes) != 1 || !outcomes[0].LeaseLost || outcomes[0].Dead || outcomes[0].Err == nil {
			t.Errorf("outcomes = %+v, want one failure with LeaseLost set", outcomes)
		}
		mu.Unlock()

		close(releaseB)
		if err := <-bDone; err != nil {
			t.Fatalf("the second relay: %v", err)
		}
		if st = states(t, db); !st[0].C3 {
			t.Fatalf("row after the holder published = %+v, want published", st)
		}
	})
}

// Catches: a batch that is published past its lease (the later events would
// then be delivered by two relays), or events left leased and counted as
// attempts when the lease ends before the batch does.
func TestBatchPastTheLeaseReleasesTheRest(t *testing.T) {
	testopen.EachWithMySQL(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		o := setup(t, db, d)
		addAll(t, db, o,
			outbox.Event{Topic: "t", Payload: []byte("1")},
			outbox.Event{Topic: "t", Payload: []byte("2")},
			outbox.Event{Topic: "t", Payload: []byte("3")},
		)
		pub := &collector{}
		first := outbox.PublisherFunc(func(ctx context.Context, e outbox.Event) error {
			_ = pub.Publish(ctx, e)
			<-ctx.Done() // the publish of the first event runs to the end of the lease
			return nil
		})
		r := newRelay(t, o, db, first, outbox.WithLease(300*time.Millisecond), outbox.WithBatchSize(3))
		if n, err := r.RunOnce(context.Background()); err != nil || n != 3 {
			t.Fatalf("RunOnce = %d, %v; want the batch of 3", n, err)
		}
		if got := pub.events(); len(got) != 1 {
			t.Fatalf("the publisher saw %d events, want only the first: %+v", len(got), got)
		}
		st := states(t, db)
		if !st[0].C3 || st[0].C2 != 1 {
			t.Errorf("first row = %+v, want published after 1 attempt", st[0])
		}
		for _, s := range st[1:] {
			if s.C2 != 0 || s.C3 || s.C4 {
				t.Errorf("row %d = %+v, want released: attempts 0, pending", s.C1, s)
			}
		}
		rest := &collector{}
		if n := drain(t, newRelay(t, o, db, rest)); n != 2 {
			t.Fatalf("a second relay claimed %d, want the 2 released events at once", n)
		}
		for _, e := range rest.events() {
			if e.Attempt != 1 {
				t.Errorf("event %d: attempt %d, want 1 (the release gave the attempt back)", e.ID, e.Attempt)
			}
		}
	})
}

// Catches: an event claimed by a relay that crashed staying lost, or being
// claimed before its lease ran out. The claim is made by hand and never
// finished, which is what a crash leaves behind.
func TestEventOfCrashedRelayIsClaimedAfterTheLease(t *testing.T) {
	testopen.EachWithMySQL(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		o := setup(t, db, d)
		ids := addAll(t, db, o, outbox.Event{Topic: "t", Payload: []byte("x")})
		claimByHand(t, db, d, ids[0], 1, time.Now().Add(400*time.Millisecond))

		pub := &collector{}
		r := newRelay(t, o, db, pub)
		if n, err := r.RunOnce(context.Background()); err != nil || n != 0 {
			t.Fatalf("RunOnce inside the lease = %d, %v; want 0, the event is leased", n, err)
		}
		time.Sleep(600 * time.Millisecond)
		if n, err := r.RunOnce(context.Background()); err != nil || n != 1 {
			t.Fatalf("RunOnce after the lease = %d, %v; want the abandoned event", n, err)
		}
		got := pub.events()
		if len(got) != 1 || got[0].ID != ids[0] || got[0].Attempt != 2 {
			t.Fatalf("the publisher saw %+v, want the event as attempt 2: the crashed attempt counted", got)
		}
		if st := states(t, db); !st[0].C3 || st[0].C2 != 2 {
			t.Errorf("row = %+v, want published after 2 attempts", st[0])
		}
	})
}

// Catches: a poison event that crashes its relay every time being claimed and
// published forever. Its last allowed attempt never ended, so the next claim
// marks it dead without publishing it, reports it, and the later event of its
// key stays held back.
func TestEventThatKeepsCrashingTheRelayBecomesDead(t *testing.T) {
	testopen.EachWithMySQL(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		o := setup(t, db, d)
		ids := addAll(t, db, o,
			outbox.Event{Topic: "t", Key: "k", Payload: []byte("poison")},
			outbox.Event{Topic: "t", Key: "k", Payload: []byte("after")},
		)
		claimByHand(t, db, d, ids[0], 3, time.Now().Add(-time.Second)) // the third and last attempt never ended

		pub := &collector{}
		log := &errorLog{}
		var mu sync.Mutex
		var outcomes []outbox.Outcome
		r := newRelay(t, o, db, pub, outbox.WithMaxAttempts(3), outbox.WithOnError(log.add),
			outbox.WithObserver(func(oc outbox.Outcome) {
				mu.Lock()
				outcomes = append(outcomes, oc)
				mu.Unlock()
			}))
		if n, err := r.RunOnce(context.Background()); err != nil || n != 1 {
			t.Fatalf("RunOnce = %d, %v; want the poison event claimed", n, err)
		}
		if got := pub.events(); len(got) != 0 {
			t.Fatalf("the publisher saw %+v, want nothing: a dead event is not published", got)
		}
		st := states(t, db)
		if !st[0].C4 || st[0].C3 || st[0].C5 == nil || !strings.Contains(*st[0].C5, "never ended") {
			t.Errorf("poison row = %+v, want dead with the reason kept", st[0])
		}
		if st[1].C2 != 0 || st[1].C3 || st[1].C4 {
			t.Errorf("later row = %+v, want untouched behind the dead event", st[1])
		}
		if errs := log.all(); len(errs) != 1 || !strings.Contains(errs[0].Error(), "never ended") {
			t.Errorf("WithOnError got %v, want one report of the abandoned event", errs)
		}
		mu.Lock()
		if len(outcomes) != 1 || !outcomes[0].Dead || outcomes[0].Err == nil || outcomes[0].LeaseLost {
			t.Errorf("outcomes = %+v, want one dead outcome", outcomes)
		}
		mu.Unlock()
		if n, err := r.RunOnce(context.Background()); err != nil || n != 0 {
			t.Errorf("RunOnce after the abandon = %d, %v; want 0: the dead event holds back its key", n, err)
		}
	})
}

// Catches: a publisher panic that ends the relay or goes unreported, or one that
// is not counted as a failed attempt. The panic and its stack must reach
// WithOnError.
func TestPublisherPanicIsAFailedAttempt(t *testing.T) {
	testopen.EachWithMySQL(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		o := setup(t, db, d)
		addAll(t, db, o, outbox.Event{Topic: "t", Payload: []byte("x")})
		log := &errorLog{}
		r := newRelay(t, o, db, outbox.PublisherFunc(func(context.Context, outbox.Event) error {
			panic("kaboom")
		}), outbox.WithOnError(log.add))
		if n, err := r.RunOnce(context.Background()); err != nil || n != 1 {
			t.Fatalf("RunOnce = %d, %v; want 1, nil: the panic is recovered", n, err)
		}
		st := states(t, db)
		if st[0].C2 != 1 || st[0].C3 || st[0].C4 || st[0].C5 == nil || !strings.Contains(*st[0].C5, "panicked: kaboom") {
			t.Errorf("row = %+v, want one failed attempt with the panic as the error", st[0])
		}
		errs := log.all()
		if len(errs) != 1 || !strings.Contains(errs[0].Error(), "kaboom") || !strings.Contains(errs[0].Error(), "goroutine") {
			t.Errorf("WithOnError got %v, want the panic with its stack", errs)
		}
	})
}

// Catches: headers that are not a JSON object of strings being dropped without
// a word.
func TestUnparsableHeadersAreReported(t *testing.T) {
	testopen.EachWithMySQL(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		o := setup(t, db, d)
		ids := addAll(t, db, o, outbox.Event{Topic: "t", Payload: []byte("x")})
		if _, err := momo.Exec(context.Background(), db, `UPDATE outbox_events SET headers = ? WHERE id = ?`, "not json", ids[0]); err != nil {
			t.Fatal(err)
		}
		pub := &collector{}
		log := &errorLog{}
		if n, err := newRelay(t, o, db, pub, outbox.WithOnError(log.add)).RunOnce(context.Background()); err != nil || n != 1 {
			t.Fatalf("RunOnce = %d, %v; want 1, nil", n, err)
		}
		if got := pub.events(); len(got) != 1 || got[0].Headers != nil {
			t.Errorf("the publisher saw %+v, want the event without headers", got)
		}
		if errs := log.all(); len(errs) != 1 || !strings.Contains(errs[0].Error(), "headers") {
			t.Errorf("WithOnError got %v, want one report about the headers", errs)
		}
	})
}
