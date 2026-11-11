// SPDX-License-Identifier: Apache-2.0

//go:build integration

package pgxdriver

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb"
	"github.com/jackc/pgx/v5"
)

// listenWait bounds every wait on a channel. It is generous on purpose: a
// passing test never waits this long, and a failing one must not hang.
const listenWait = 30 * time.Second

// uniqueChannel returns a channel name no other test uses: LISTEN channels are
// per database, not per schema.
func uniqueChannel() string {
	return fmt.Sprintf("momo_listen_%d", time.Now().UnixNano())
}

type waitResult struct {
	n   Notification
	err error
}

// pump runs l.Wait in a loop on its own goroutine and sends every result on
// the returned channel until Wait returns an error.
func pump(ctx context.Context, l *Listener) <-chan waitResult {
	out := make(chan waitResult, 16)
	go func() {
		defer close(out)
		for {
			n, err := l.Wait(ctx)
			out <- waitResult{n, err}
			if err != nil {
				return
			}
		}
	}()
	return out
}

func recv[T any](t *testing.T, ch <-chan T, what string) T {
	t.Helper()
	select {
	case v, ok := <-ch:
		if !ok {
			t.Fatalf("%s: channel closed", what)
		}
		return v
	case <-time.After(listenWait):
		t.Fatalf("timed out waiting for %s", what)
		panic("unreachable")
	}
}

func connCfg(t *testing.T) *pgx.ConnConfig {
	t.Helper()
	cfg, err := pgx.ParseConfig(testdb.PostgresURL(t))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// newListener starts a listener and a goroutine pumping its Wait results.
func newListener(t *testing.T, channels []string, opts ...ListenOption) (*Listener, <-chan waitResult, context.Context) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*listenWait)
	t.Cleanup(cancel)
	l, err := Listen(ctx, connCfg(t), channels, opts...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), listenWait)
		defer ccancel()
		_ = l.Close(cctx)
	})
	return l, pump(ctx, l), ctx
}

func TestListenNotifyDeliveredAtCommitNotAtRollback(t *testing.T) {
	ch := uniqueChannel()
	_, results, ctx := newListener(t, []string{ch})
	db := momo.New(New(openPool(t)))

	// Rolled back: never delivered.
	tx, err := db.Begin(ctx, momo.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := Notify(ctx, tx, ch, "rolled-back"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}

	// Committed: delivered.
	tx, err = db.Begin(ctx, momo.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := Notify(ctx, tx, ch, "committed"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// The first notification to arrive must be the committed one: the
	// rolled-back one was sent first, so had it been delivered it would
	// have come first.
	r := recv(t, results, "committed notification")
	if r.err != nil {
		t.Fatal(r.err)
	}
	if r.n.Channel != ch || r.n.Payload != "committed" || r.n.PID == 0 {
		t.Fatalf("got %+v, want channel %q payload committed and a PID", r.n, ch)
	}
	// Autocommit delivers at once; receiving it also proves nothing was
	// queued behind the committed one.
	if err := Notify(ctx, db, ch, "after"); err != nil {
		t.Fatal(err)
	}
	r = recv(t, results, "autocommit notification")
	if r.err != nil || r.n.Payload != "after" {
		t.Fatalf("got %+v, %v, want payload after", r.n, r.err)
	}
}

func TestListenLargePayload(t *testing.T) {
	ch := uniqueChannel()
	_, results, ctx := newListener(t, []string{ch})
	db := momo.New(New(openPool(t)))

	// 7999 bytes is the longest payload PostgreSQL accepts (it rejects 8000).
	payload := strings.Repeat("p", 7999)
	if err := Notify(ctx, db, ch, payload); err != nil {
		t.Fatal(err)
	}
	r := recv(t, results, "large notification")
	if r.err != nil {
		t.Fatal(r.err)
	}
	if r.n.Payload != payload {
		t.Fatalf("payload length %d, want %d", len(r.n.Payload), len(payload))
	}
	if err := Notify(ctx, db, ch, strings.Repeat("p", 8000)); err == nil {
		t.Fatal("8000-byte payload: want the server's too-long error")
	}
}

func TestListenMultipleAndQuotedChannels(t *testing.T) {
	base := uniqueChannel()
	odd := base + ` Mixed"Case`
	_, results, ctx := newListener(t, []string{base, odd})
	db := momo.New(New(openPool(t)))
	for _, ch := range []string{odd, base} {
		if err := Notify(ctx, db, ch, "x"); err != nil {
			t.Fatal(err)
		}
		r := recv(t, results, "notification on "+ch)
		if r.err != nil || r.n.Channel != ch {
			t.Fatalf("got %+v, %v, want channel %q", r.n, r.err, ch)
		}
	}
	if err := Notify(ctx, db, "bad\x00name", "x"); err == nil {
		t.Fatal("NUL in channel: want error")
	}
}

func TestListenReconnectsAfterBackendTermination(t *testing.T) {
	ch := uniqueChannel()
	reconnected := make(chan struct{}, 4)
	_, results, ctx := newListener(t, []string{ch},
		OnReconnect(func() { reconnected <- struct{}{} }),
		WithBackoff(10*time.Millisecond, 200*time.Millisecond))
	pool := openPool(t)
	db := momo.New(New(pool))

	// The listener's own connection runs nothing after LISTEN, so its
	// pg_stat_activity row still shows that statement. Finding the backend
	// through the server avoids reading the Listener's connection while Wait
	// uses it.
	var pid int32
	if err := pool.QueryRow(ctx,
		`SELECT pid FROM pg_stat_activity WHERE query = $1 AND pid <> pg_backend_pid()`,
		"LISTEN "+quoteChannel(ch)).Scan(&pid); err != nil {
		t.Fatalf("find listener backend: %v", err)
	}
	var terminated bool
	if err := pool.QueryRow(ctx, "SELECT pg_terminate_backend($1)", pid).Scan(&terminated); err != nil || !terminated {
		t.Fatalf("pg_terminate_backend(%d) = %v, %v", pid, terminated, err)
	}

	recv(t, reconnected, "OnReconnect")
	// OnReconnect runs after LISTEN is re-issued, so this is delivered.
	if err := Notify(ctx, db, ch, "after-reconnect"); err != nil {
		t.Fatal(err)
	}
	r := recv(t, results, "notification after reconnect")
	if r.err != nil || r.n.Payload != "after-reconnect" {
		t.Fatalf("got %+v, %v, want payload after-reconnect", r.n, r.err)
	}
}

func TestListenCloseUnblocksWait(t *testing.T) {
	ch := uniqueChannel()
	l, results, _ := newListener(t, []string{ch})
	cctx, cancel := context.WithTimeout(context.Background(), listenWait)
	defer cancel()
	if err := l.Close(cctx); err != nil {
		t.Fatal(err)
	}
	r := recv(t, results, "Wait result after Close")
	if !errors.Is(r.err, ErrListenerClosed) {
		t.Fatalf("Wait after Close = %v, want ErrListenerClosed", r.err)
	}
	if _, err := l.Wait(cctx); !errors.Is(err, ErrListenerClosed) {
		t.Fatalf("second Wait = %v, want ErrListenerClosed", err)
	}
	if err := l.Close(cctx); err != nil {
		t.Fatalf("second Close = %v", err)
	}
}

func TestListenWaitDeadlineThenResume(t *testing.T) {
	ch := uniqueChannel()
	reconnected := make(chan struct{}, 4)
	ctx, cancel := context.WithTimeout(context.Background(), 3*listenWait)
	defer cancel()
	l, err := Listen(ctx, connCfg(t), []string{ch}, OnReconnect(func() { reconnected <- struct{}{} }))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close(ctx) }()

	// Nothing is sent, so this Wait can only end by its deadline.
	short, stop := context.WithTimeout(ctx, 50*time.Millisecond)
	defer stop()
	if _, err := l.Wait(short); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Wait = %v, want context.DeadlineExceeded", err)
	}

	// The next Wait reconnects (Wait drops a connection a failed read
	// touched), reports it, and delivers again.
	results := pump(ctx, l)
	recv(t, reconnected, "OnReconnect after deadline")
	db := momo.New(New(openPool(t)))
	if err := Notify(ctx, db, ch, "resumed"); err != nil {
		t.Fatal(err)
	}
	r := recv(t, results, "notification after deadline")
	if r.err != nil || r.n.Payload != "resumed" {
		t.Fatalf("got %+v, %v, want payload resumed", r.n, r.err)
	}
}
