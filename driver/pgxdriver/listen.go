// SPDX-License-Identifier: Apache-2.0

package pgxdriver

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/jackc/pgx/v5"
)

// ErrListenerClosed is returned by Wait once Close has been called.
var ErrListenerClosed = errors.New("pgxdriver: listener closed")

// maxChannelBytes is PostgreSQL's identifier limit (NAMEDATALEN - 1).
const maxChannelBytes = 63

// Notification is one NOTIFY delivered to a Listener.
type Notification struct {
	// Channel is the channel the notification was sent on.
	Channel string
	// Payload is the payload string; empty when the sender gave none.
	Payload string
	// PID is the process ID of the notifying backend.
	PID uint32
}

// ListenOption configures Listen.
type ListenOption func(*listenConfig)

type listenConfig struct {
	onReconnect  func()
	minBackoff   time.Duration
	maxBackoff   time.Duration
	backoffAdded bool
}

// OnReconnect sets fn to run each time the listener has re-established its
// connection and re-issued LISTEN after losing it. Notifications sent while
// it was disconnected are lost, so treat every call as "something may have
// been missed" and resynchronize. fn runs on the goroutine that called Wait,
// before Wait delivers anything from the new connection.
func OnReconnect(fn func()) ListenOption {
	return func(c *listenConfig) { c.onReconnect = fn }
}

// WithBackoff sets the delay between failed reconnect attempts. The delay
// starts at lo and doubles up to hi. The first attempt after a lost
// connection is immediate. The default is 100ms to 10s. Listen returns an
// error when lo is not positive or hi is below lo.
func WithBackoff(lo, hi time.Duration) ListenOption {
	return func(c *listenConfig) {
		c.minBackoff, c.maxBackoff, c.backoffAdded = lo, hi, true
	}
}

// Listener receives PostgreSQL LISTEN/NOTIFY notifications on one dedicated,
// non-pooled connection. It is PostgreSQL only and is not tied to a pool: a
// LISTEN session cannot go through PgBouncer in transaction mode, so connect
// straight to the server. Wait may be called from one goroutine at a time;
// Close may be called from any goroutine.
type Listener struct {
	cfg      *pgx.ConnConfig
	channels []string
	opts     listenConfig

	// sem serializes Wait and Close (a one-slot semaphore so it can be
	// waited on with a context). conn is only touched while holding it.
	sem  chan struct{}
	conn *pgx.Conn

	closeCtx context.Context
	closeFn  context.CancelFunc
}

// Listen connects with connConfig, runs LISTEN on every channel and returns a
// Listener. The initial connection and LISTEN happen before Listen returns, so
// a nil error means the listener is subscribed. connConfig is copied; a nil
// config, an empty channel list or an invalid channel name is an error.
func Listen(ctx context.Context, connConfig *pgx.ConnConfig, channels []string, opts ...ListenOption) (*Listener, error) {
	if connConfig == nil {
		return nil, errors.New("pgxdriver: Listen needs a connection config")
	}
	if len(channels) == 0 {
		return nil, errors.New("pgxdriver: Listen needs at least one channel")
	}
	cfg := listenConfig{minBackoff: 100 * time.Millisecond, maxBackoff: 10 * time.Second}
	for _, o := range opts {
		o(&cfg)
	}
	if cfg.backoffAdded && (cfg.minBackoff <= 0 || cfg.maxBackoff < cfg.minBackoff) {
		return nil, fmt.Errorf("pgxdriver: WithBackoff(%v, %v): min must be positive and max at least min", cfg.minBackoff, cfg.maxBackoff)
	}
	for _, ch := range channels {
		if err := validateChannel(ch); err != nil {
			return nil, err
		}
	}
	l := &Listener{
		cfg:      connConfig.Copy(),
		channels: append([]string(nil), channels...),
		opts:     cfg,
		sem:      make(chan struct{}, 1),
	}
	l.closeCtx, l.closeFn = context.WithCancel(context.Background())
	conn, err := l.dial(ctx)
	if err != nil {
		l.closeFn()
		return nil, err
	}
	l.conn = conn
	return l, nil
}

// Wait blocks until a notification arrives and returns it. When the
// connection is lost, Wait reconnects with backoff, re-issues LISTEN, calls
// the OnReconnect function, and keeps waiting. It returns the context's error
// when ctx ends (the next Wait reconnects, because a cancelled read leaves the
// connection unusable) and ErrListenerClosed after Close.
func (l *Listener) Wait(ctx context.Context) (Notification, error) {
	select {
	case l.sem <- struct{}{}:
	case <-ctx.Done():
		return Notification{}, ctx.Err()
	case <-l.closeCtx.Done():
		return Notification{}, ErrListenerClosed
	}
	defer func() { <-l.sem }()

	// A context that ends when the caller's does or when Close runs.
	wctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(l.closeCtx, cancel)
	defer stop()

	var delay time.Duration
	for {
		if l.closeCtx.Err() != nil {
			return Notification{}, ErrListenerClosed
		}
		if l.conn == nil {
			conn, err := l.dial(wctx)
			if err != nil {
				if werr := l.waitErr(ctx); werr != nil {
					return Notification{}, werr
				}
				delay = nextDelay(delay, l.opts)
				if !sleep(wctx, delay) {
					return Notification{}, l.waitErr(ctx)
				}
				continue
			}
			l.conn = conn
			delay = 0
			if l.opts.onReconnect != nil {
				l.opts.onReconnect()
			}
		}
		n, err := l.conn.WaitForNotification(wctx)
		if err == nil {
			return Notification{Channel: n.Channel, Payload: n.Payload, PID: n.PID}, nil
		}
		// Any failure leaves the connection unusable: drop it so the next
		// pass reconnects and treats it as a possible gap.
		l.dropConn()
		if werr := l.waitErr(ctx); werr != nil {
			return Notification{}, werr
		}
	}
}

// Close stops the listener, interrupts a blocked Wait (which returns
// ErrListenerClosed) and closes the connection. It is safe to call more than
// once. ctx bounds the wait for the connection to close.
func (l *Listener) Close(ctx context.Context) error {
	l.closeFn()
	select {
	case l.sem <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-l.sem }()
	if l.conn == nil {
		return nil
	}
	err := l.conn.Close(ctx)
	l.conn = nil
	return err
}

// waitErr reports why Wait must stop: Close or the caller's context.
func (l *Listener) waitErr(ctx context.Context) error {
	if l.closeCtx.Err() != nil {
		return ErrListenerClosed
	}
	return ctx.Err()
}

func (l *Listener) dropConn() {
	if l.conn == nil {
		return
	}
	closeNow(l.conn)
	l.conn = nil
}

// closeNow closes c with an already-expired context so it never blocks on a
// dead peer.
func closeNow(c *pgx.Conn) {
	dead, cancel := context.WithCancel(context.Background())
	cancel()
	_ = c.Close(dead)
}

// dial opens a connection and subscribes it to every channel.
func (l *Listener) dial(ctx context.Context) (*pgx.Conn, error) {
	conn, err := pgx.ConnectConfig(ctx, l.cfg)
	if err != nil {
		return nil, err
	}
	for _, ch := range l.channels {
		if _, err := conn.Exec(ctx, listenStatement(ch)); err != nil {
			closeNow(conn)
			return nil, fmt.Errorf("pgxdriver: LISTEN %q: %w", ch, err)
		}
	}
	return conn, nil
}

func nextDelay(cur time.Duration, c listenConfig) time.Duration {
	if cur <= 0 {
		return c.minBackoff
	}
	cur *= 2
	if cur > c.maxBackoff {
		return c.maxBackoff
	}
	return cur
}

// sleep waits d or until ctx ends; it reports whether the full delay passed.
func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// Notify sends payload on channel with SELECT pg_notify($1, $2). Inside a
// transaction the notification is delivered at commit and dropped on
// rollback; outside one it is delivered at once. PostgreSQL rejects a payload
// of 8000 bytes or more, so the longest accepted payload is 7999 bytes; the
// server's error is returned unchanged.
func Notify(ctx context.Context, q momo.Querier, channel, payload string) error {
	if err := validateChannel(channel); err != nil {
		return err
	}
	_, err := momo.Exec(ctx, q, "SELECT pg_notify($1, $2)", channel, payload)
	return err
}

func validateChannel(ch string) error {
	switch {
	case ch == "":
		return errors.New("pgxdriver: channel name is empty")
	case len(ch) > maxChannelBytes:
		return fmt.Errorf("pgxdriver: channel name is %d bytes, PostgreSQL allows %d", len(ch), maxChannelBytes)
	case strings.ContainsRune(ch, 0):
		return errors.New("pgxdriver: channel name contains a NUL byte")
	}
	return nil
}

// listenStatement returns LISTEN for ch. LISTEN takes an identifier, not a
// bind parameter, so the name is validated by the caller and quoted here.
func listenStatement(ch string) string {
	return fmt.Sprintf("LISTEN %s", quoteChannel(ch))
}

// quoteChannel quotes ch as a case-preserving identifier, the form
// pg_notify's channel argument matches.
func quoteChannel(ch string) string {
	return pgx.Identifier{ch}.Sanitize()
}
