// SPDX-License-Identifier: Apache-2.0

package outbox

import (
	"context"
	"fmt"
	"time"

	"github.com/azrtydxb/momo/driver/pgxdriver"
	"github.com/jackc/pgx/v5"
)

const (
	// listenRetryMin and listenRetryMax bound the wait between attempts to
	// start the listener.
	listenRetryMin = time.Second
	listenRetryMax = 30 * time.Second
	// listenCloseTimeout bounds closing the listener connection at the end of
	// Run.
	listenCloseTimeout = 5 * time.Second
)

// WithListen makes Run wake on the notifications Add sends on PostgreSQL, so a
// committed event is published within a round trip instead of within a poll
// interval. It listens on its own connection, opened with cfg, through
// pgxdriver.Listen: connect to the server directly, or through PgBouncer in
// session mode, never in transaction mode. Polling stays on as the fallback: a
// notification that was lost while the connection was down is made up for by a
// pass at every reconnect and by the poll interval, so the poll interval can be
// long but should not be infinite.
//
// If the listener cannot connect, Run reports the error to the WithOnError
// function, keeps polling, and retries the connection with a growing delay.
// WithListen on a database other than PostgreSQL makes NewRelay fail with
// momo.ErrUnsupported, and a nil cfg with momo.ErrValidation.
func WithListen(cfg *pgx.ConnConfig) RelayOption {
	return func(c *relayConfig) {
		c.listen = cfg
		c.listenSet = true
	}
}

// startListener starts the goroutine that turns notifications into wake-ups,
// and returns the function that stops it and waits for it. Without WithListen
// it does nothing.
func (r *Relay) startListener(ctx context.Context) (stop func()) {
	if r.cfg.listen == nil {
		return func() {}
	}
	lctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		r.listen(lctx)
	}()
	return func() {
		cancel()
		<-done
	}
}

// pump wakes the relay for every notification of l until ctx ends, and closes l.
func (r *Relay) pump(ctx context.Context, l *pgxdriver.Listener) {
	defer func() {
		cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), listenCloseTimeout)
		defer cancel()
		_ = l.Close(cctx)
	}()
	// An event added before the subscription was in place sent a notification
	// nobody heard.
	r.Wake()
	for {
		if _, err := l.Wait(ctx); err != nil {
			return
		}
		r.Wake()
	}
}

// listen connects the listener, retrying with a growing delay, and wakes the
// relay for every notification until ctx ends.
func (r *Relay) listen(ctx context.Context) {
	wait := listenRetryMin
	for {
		l, err := pgxdriver.Listen(ctx, r.cfg.listen, []string{r.o.table}, pgxdriver.WithOnReconnect(r.Wake))
		if err == nil {
			r.pump(ctx, l)
			return
		}
		if ctx.Err() != nil {
			return
		}
		r.cfg.onError(fmt.Errorf("outbox: LISTEN failed, polling instead: %w", err))
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		}
		wait = min(wait*2, listenRetryMax)
	}
}
