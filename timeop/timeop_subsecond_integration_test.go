// SPDX-License-Identifier: Apache-2.0

//go:build integration

package timeop_test

import (
	"math"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/timeop"
)

// Catches: a lease or TTL of 1500ms rounded to whole seconds, a millisecond
// unit the server rejects, and a bound millisecond count that is scaled wrong.
func TestSubSecondIntervalIntegration(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		seedTimeDocs(t, db)
		at := time.Date(2024, 3, 15, 13, 45, 30, 0, time.UTC)

		if got := timeOf(t, db, 1, timeop.AddInterval(suite.DocCreatedAt, timeop.IntervalOf(1500, timeop.IntervalMillisecond))); !got.Equal(at.Add(1500 * time.Millisecond)) {
			t.Errorf("+1500 milliseconds = %v, want %v", got, at.Add(1500*time.Millisecond))
		}
		if got := timeOf(t, db, 1, timeop.SubInterval(suite.DocCreatedAt, timeop.IntervalOf(250, timeop.IntervalMillisecond))); !got.Equal(at.Add(-250 * time.Millisecond)) {
			t.Errorf("-250 milliseconds = %v", got)
		}
		bound := timeop.IntervalExpr(momo.Value(int64(2750)), timeop.IntervalMillisecond)
		if got := timeOf(t, db, 1, timeop.AddInterval(suite.DocCreatedAt, bound)); !got.Equal(at.Add(2750 * time.Millisecond)) {
			t.Errorf("+2750 bound milliseconds = %v", got)
		}
		if got := timeOf(t, db, 1, timeop.AddDuration(suite.DocCreatedAt, 1500*time.Millisecond)); !got.Equal(at.Add(1500 * time.Millisecond)) {
			t.Errorf("AddDuration 1500ms = %v", got)
		}
		if got := timeOf(t, db, 1, timeop.SubDuration(suite.DocCreatedAt, 90*time.Second)); !got.Equal(at.Add(-90 * time.Second)) {
			t.Errorf("SubDuration 90s = %v", got)
		}
		if d != dialect.SQLite {
			// SQLite timestamps stop at the millisecond.
			if got := timeOf(t, db, 1, timeop.AddInterval(suite.DocCreatedAt, timeop.IntervalOf(250, timeop.IntervalMicrosecond))); !got.Equal(at.Add(250 * time.Microsecond)) {
				t.Errorf("+250 microseconds = %v", got)
			}
			if got := timeOf(t, db, 1, timeop.AddDuration(suite.DocCreatedAt, 1500*time.Microsecond)); !got.Equal(at.Add(1500 * time.Microsecond)) {
				t.Errorf("AddDuration 1500us = %v", got)
			}
		}
	})
}

// Catches: a difference with its operands swapped, an epoch that loses the
// fraction, and an interval value that cannot be scanned as seconds.
func TestTimestampDifferenceIntegration(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		seedTimeDocs(t, db)
		created := momo.Expression[time.Time](suite.DocCreatedAt)
		later := timeop.AddDuration(suite.DocCreatedAt, 1500*time.Millisecond)

		near := func(name string, got, want float64) {
			t.Helper()
			if math.Abs(got-want) > 1e-3 {
				t.Errorf("%s = %v, want %v", name, got, want)
			}
		}
		near("later - created", floatOf(t, db, 1, timeop.EpochSeconds(timeop.Sub(later, created))), 1.5)
		near("created - later", floatOf(t, db, 1, timeop.EpochSeconds(timeop.Sub(created, later))), -1.5)
		hours := timeop.SubInterval(suite.DocCreatedAt, timeop.IntervalOf(3, timeop.IntervalHour))
		near("created - 3h", floatOf(t, db, 1, timeop.EpochSeconds(timeop.Sub(created, hours))), 3*3600)
		near("self", floatOf(t, db, 1, timeop.EpochSeconds(timeop.Sub(created, created))), 0)

		if d == dialect.Postgres {
			near("age 3h", floatOf(t, db, 1, timeop.EpochSeconds(timeop.Age(created, hours))), 3*3600)
			bound := time.Date(2024, 3, 15, 13, 45, 0, 0, time.UTC)
			near("bound", floatOf(t, db, 1, timeop.EpochSeconds(timeop.Sub(created, momo.Value(bound)))), 30)
		}
	})
}
