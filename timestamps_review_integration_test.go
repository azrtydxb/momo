// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

func TestClockNormalizesToUTCAndRoundTripsOnPostgres(t *testing.T) {
	// A non-UTC clock with nanoseconds. Postgres stores microseconds, so the
	// stored value equals the clock truncated to the microsecond; the instant
	// is unchanged by the zone.
	zone := time.FixedZone("UTC+5", 5*3600)
	at := time.Date(2031, 6, 7, 18, 4, 5, 123456789, zone)
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		if d != dialect.Postgres {
			t.Skip("microsecond truncation is a PostgreSQL storage property")
		}
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		cdb := momo.New(db.Driver(), momo.WithClock(func() time.Time { return at }))
		doc := suite.Document{TenantID: 1, Title: "tz"}
		if _, err := momo.Insert[suite.Document](cdb).Model(&doc).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		got, err := momo.Select[suite.Document](db).Where(suite.DocTitle.Eq("tz")).One(ctx)
		if err != nil {
			t.Fatal(err)
		}
		want := at.UTC().Truncate(time.Microsecond)
		if !got.CreatedAt.Equal(want) || !got.UpdatedAt.Equal(want) {
			t.Fatalf("stored %v %v, want %v", got.CreatedAt, got.UpdatedAt, want)
		}
	})

	c, err := momo.Insert[suite.Document](momo.New(fakedriver.New(dialect.Postgres), momo.WithClock(func() time.Time { return at }))).
		Values(suite.DocTenantID.Set(1)).SQL()
	if err != nil {
		t.Fatal(err)
	}
	var stamps int
	for _, a := range c.Args {
		ts, ok := a.(time.Time)
		if !ok {
			continue
		}
		stamps++
		if ts.Location() != time.UTC || !ts.Equal(at) {
			t.Fatalf("bound timestamp %v (location %v), want %v in UTC", ts, ts.Location(), at)
		}
	}
	if stamps != 2 {
		t.Fatalf("bound %d timestamps, want created_at and updated_at: %v", stamps, c.Args)
	}
}

func TestClockReachesTransactionInsert(t *testing.T) {
	fixed := time.Date(2032, 3, 4, 5, 6, 7, 0, time.UTC)
	testopen.Each(t, func(t *testing.T, db *momo.DB, _ dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		cdb := momo.New(db.Driver(), clockAt(fixed))
		tx, err := cdb.Begin(ctx, momo.TxOptions{})
		if err != nil {
			t.Fatal(err)
		}
		doc := suite.Document{TenantID: 1, Title: "in-tx"}
		if _, err := momo.Insert[suite.Document](tx).Model(&doc).Exec(ctx); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		got, err := momo.Select[suite.Document](db).Where(suite.DocTitle.Eq("in-tx")).One(ctx)
		if err != nil || !got.CreatedAt.Equal(fixed) || !got.UpdatedAt.Equal(fixed) {
			t.Fatalf("tx insert timestamps = %v %v (%v), want %v", got.CreatedAt, got.UpdatedAt, err, fixed)
		}
	})
}
