// SPDX-License-Identifier: Apache-2.0

// Package outbox is a transactional outbox (stub for the red run).
package outbox

import (
	"context"
	"errors"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/jackc/pgx/v5"
)

// DefaultTable is the default table name.
const DefaultTable = "outbox_events"

var errNotImplemented = errors.New("outbox: not implemented")

// Event is one event.
type Event struct {
	ID      int64
	Topic   string
	Key     string
	Payload []byte
	Headers map[string]string
	Attempt int
}

// Publisher publishes one event.
type Publisher interface {
	Publish(ctx context.Context, e Event) error
}

// PublisherFunc adapts a function to a Publisher.
type PublisherFunc func(ctx context.Context, e Event) error

// Publish calls f.
func (f PublisherFunc) Publish(ctx context.Context, e Event) error { return f(ctx, e) }

// Outcome is the result of one delivery.
type Outcome struct {
	Event    Event
	Err      error
	Dead     bool
	Duration time.Duration
}

// Outbox is the outbox table.
type Outbox struct{}

// Option configures an Outbox.
type Option func(*Outbox)

// WithTable names the table.
func WithTable(name string) Option { return func(*Outbox) {} }

// WithoutNotify turns off NOTIFY.
func WithoutNotify() Option { return func(*Outbox) {} }

// New returns an Outbox.
func New(opts ...Option) (*Outbox, error) { return &Outbox{}, nil }

// Table returns the table name.
func (o *Outbox) Table() string { return "" }

// Add writes an event.
func (o *Outbox) Add(ctx context.Context, tx *momo.Tx, e Event) (int64, error) {
	return 0, errNotImplemented
}

// Cleanup deletes old published events.
func (o *Outbox) Cleanup(ctx context.Context, q momo.Querier, retention time.Duration) (int64, error) {
	return 0, errNotImplemented
}

// Requeue makes dead events pending again.
func (o *Outbox) Requeue(ctx context.Context, q momo.Querier, ids ...int64) (int64, error) {
	return 0, errNotImplemented
}

// Relay publishes events.
type Relay struct{}

// RelayOption configures a Relay.
type RelayOption func(*Relay)

// WithBatchSize sets the batch size.
func WithBatchSize(n int) RelayOption { return func(*Relay) {} }

// WithLease sets the lease.
func WithLease(d time.Duration) RelayOption { return func(*Relay) {} }

// WithPollInterval sets the poll interval.
func WithPollInterval(d time.Duration) RelayOption { return func(*Relay) {} }

// WithBackoff sets the retry backoff.
func WithBackoff(base, max time.Duration) RelayOption { return func(*Relay) {} }

// WithMaxAttempts sets the attempt limit.
func WithMaxAttempts(n int) RelayOption { return func(*Relay) {} }

// WithRetention sets the retention.
func WithRetention(d time.Duration) RelayOption { return func(*Relay) {} }

// WithObserver sets the observer.
func WithObserver(fn func(Outcome)) RelayOption { return func(*Relay) {} }

// WithOnError sets the error handler.
func WithOnError(fn func(error)) RelayOption { return func(*Relay) {} }

// WithListen wakes the relay on NOTIFY.
func WithListen(cfg *pgx.ConnConfig) RelayOption { return func(*Relay) {} }

// NewRelay returns a relay.
func (o *Outbox) NewRelay(db *momo.DB, p Publisher, opts ...RelayOption) (*Relay, error) {
	return &Relay{}, nil
}

// Run runs the relay.
func (r *Relay) Run(ctx context.Context) error { return errNotImplemented }

// RunOnce delivers one batch.
func (r *Relay) RunOnce(ctx context.Context) (int, error) { return 0, errNotImplemented }

// Wake wakes the relay.
func (r *Relay) Wake() {}
