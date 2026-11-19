// SPDX-License-Identifier: Apache-2.0

package outbox_test

import (
	"context"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/outbox"
)

// Catches: a retry delay without jitter (events that failed together retry
// together), a jitter wider than 20 percent, and a delay past the maximum.
func TestBackoffDoublesWithJitterAndNeverPassesTheMaximum(t *testing.T) {
	o, err := outbox.New()
	if err != nil {
		t.Fatal(err)
	}
	db := momo.New(fakedriver.New(dialect.Postgres))
	pub := outbox.PublisherFunc(func(context.Context, outbox.Event) error { return nil })
	build := func(rnd float64) *outbox.Relay {
		r, err := o.NewRelay(db, pub, outbox.WithBackoff(time.Second, 10*time.Second), outbox.WithRand(func() float64 { return rnd }))
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	for _, tc := range []struct {
		rnd     float64
		attempt int
		want    time.Duration
	}{
		{0.5, 1, time.Second},
		{0.5, 2, 2 * time.Second},
		{0.5, 3, 4 * time.Second},
		{0, 1, 800 * time.Millisecond},
		{0, 3, 3200 * time.Millisecond},
		{0.999999, 1, 1199999 * time.Microsecond},
		{0.5, 20, 10 * time.Second},
		{0, 20, 8 * time.Second},
		{0.999999, 20, 10 * time.Second},
	} {
		got := build(tc.rnd).Backoff(tc.attempt)
		if d := got - tc.want; d < -time.Millisecond || d > time.Millisecond {
			t.Errorf("rand %v, attempt %d: delay = %v, want %v", tc.rnd, tc.attempt, got, tc.want)
		}
	}
}
