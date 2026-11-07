// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

func TestAutoTimestamps(t *testing.T) {
	fixed := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	later := fixed.Add(time.Hour)
	testopen.Each(t, func(t *testing.T, db *momo.DB, _ dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		db1 := momo.New(db.Driver(), clockAt(fixed))
		db2 := momo.New(db.Driver(), clockAt(later))
		explicit := time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC)

		if _, err := momo.Insert[suite.Document](db1).Values(suite.DocTenantID.Set(1), suite.DocTitle.Set("ts")).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		d, err := momo.Select[suite.Document](db1).Where(suite.DocTitle.Eq("ts")).One(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !d.CreatedAt.Equal(fixed) || !d.UpdatedAt.Equal(fixed) {
			t.Fatalf("insert timestamps = %v %v, want %v", d.CreatedAt, d.UpdatedAt, fixed)
		}

		// A guarded update that matches no row changes nothing.
		res, err := momo.Update[suite.Document](db2).Set(suite.DocTitle.Set("ts2")).Where(suite.DocTitle.Eq("nomatch")).Exec(ctx)
		if err != nil || res.RowsAffected != 0 {
			t.Fatalf("no-match update: %+v %v", res, err)
		}
		d2, err := momo.Select[suite.Document](db1).Where(suite.DocTitle.Eq("ts")).One(ctx)
		if err != nil || !d2.UpdatedAt.Equal(fixed) {
			t.Fatalf("no-match update changed updated_at to %v (%v)", d2.UpdatedAt, err)
		}

		// A matching update sets updated_at only.
		if _, err := momo.Update[suite.Document](db2).Set(suite.DocTitle.Set("ts2")).Where(suite.DocTitle.Eq("ts")).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		d3, err := momo.Select[suite.Document](db1).Where(suite.DocTitle.Eq("ts2")).One(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !d3.UpdatedAt.Equal(later) || !d3.CreatedAt.Equal(fixed) {
			t.Fatalf("update timestamps = created %v updated %v", d3.CreatedAt, d3.UpdatedAt)
		}

		// An explicit assignment on insert wins.
		if _, err := momo.Insert[suite.Document](db1).Values(suite.DocTenantID.Set(1), suite.DocTitle.Set("ex"),
			suite.DocCreatedAt.Set(explicit)).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		e, err := momo.Select[suite.Document](db1).Where(suite.DocTitle.Eq("ex")).One(ctx)
		if err != nil || !e.CreatedAt.Equal(explicit) || !e.UpdatedAt.Equal(fixed) {
			t.Fatalf("explicit insert = created %v updated %v (%v)", e.CreatedAt, e.UpdatedAt, err)
		}

		// Model insert: zero timestamps get the clock, set ones are kept.
		m := suite.Document{TenantID: 1, Title: "mz"}
		m2 := suite.Document{TenantID: 1, Title: "mx", CreatedAt: explicit, UpdatedAt: explicit}
		if _, err := momo.Insert[suite.Document](db1).Models([]suite.Document{m, m2}).Omit(suite.DocBody, suite.DocSecret).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		mz, err := momo.Select[suite.Document](db1).Where(suite.DocTitle.Eq("mz")).One(ctx)
		if err != nil || !mz.CreatedAt.Equal(fixed) || !mz.UpdatedAt.Equal(fixed) {
			t.Fatalf("model zero = %v %v (%v)", mz.CreatedAt, mz.UpdatedAt, err)
		}
		mx, err := momo.Select[suite.Document](db1).Where(suite.DocTitle.Eq("mx")).One(ctx)
		if err != nil || !mx.CreatedAt.Equal(explicit) || !mx.UpdatedAt.Equal(explicit) {
			t.Fatalf("model explicit = %v %v (%v)", mx.CreatedAt, mx.UpdatedAt, err)
		}

		// Model update advances updated_at too.
		mz.Title = "mz2"
		if _, err := momo.Update[suite.Document](db2).Model(&mz).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		mu, err := momo.Select[suite.Document](db1).Where(suite.DocTitle.Eq("mz2")).One(ctx)
		if err != nil || !mu.UpdatedAt.Equal(later) || !mu.CreatedAt.Equal(fixed) {
			t.Fatalf("model update = %v %v (%v)", mu.CreatedAt, mu.UpdatedAt, err)
		}
	})
}
