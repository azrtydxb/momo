// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

func clockAt(t time.Time) momo.Option { return momo.WithClock(func() time.Time { return t }) }

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

func TestAutoTimestampsSQL(t *testing.T) {
	fixed := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	db := momo.New(fakedriver.New(dialect.Postgres), clockAt(fixed))

	c, err := momo.Insert[suite.Document](db).Values(suite.DocTenantID.Set(1)).SQL()
	if err != nil {
		t.Fatal(err)
	}
	if want := `INSERT INTO "documents" ("tenant_id", "created_at", "updated_at") VALUES ($1, $2, $3)`; c.SQL != want {
		t.Fatalf("SQL = %s\nwant  %s", c.SQL, want)
	}
	if c.Args[1] != fixed || c.Args[2] != fixed {
		t.Fatalf("args = %v", c.Args)
	}

	// Omit leaves the database default on a model insert.
	doc := suite.Document{TenantID: 1}
	c, err = momo.Insert[suite.Document](db).Model(&doc).Omit(suite.DocBody, suite.DocSecret, suite.DocCreatedAt, suite.DocUpdatedAt).SQL()
	if err != nil || strings.Contains(c.SQL, "created_at") || strings.Contains(c.SQL, "updated_at") {
		t.Fatalf("omitted timestamps written: %s %v", c.SQL, err)
	}

	c, err = momo.Update[suite.Document](db).Set(suite.DocTitle.Set("x")).Where(suite.DocID.Eq(1)).SQL()
	if err != nil {
		t.Fatal(err)
	}
	if want := `UPDATE "documents" SET "title" = $1, "updated_at" = $2 WHERE "documents"."id" = $3`; c.SQL != want {
		t.Fatalf("SQL = %s\nwant  %s", c.SQL, want)
	}
	if c.Args[1] != fixed {
		t.Fatalf("args = %v", c.Args)
	}

	// The clock is read once per statement, even for multi-row inserts.
	n := 0
	cdb := momo.New(fakedriver.New(dialect.Postgres), momo.WithClock(func() time.Time { n++; return fixed }))
	rows := []suite.Document{{TenantID: 1, Title: "a"}, {TenantID: 2, Title: "b"}}
	if _, err := momo.Insert[suite.Document](cdb).Models(rows).SQL(); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("clock read %d times, want 1", n)
	}

	// An explicit Set of updated_at stays refused.
	if _, err := momo.Update[suite.Document](db).Set(suite.DocUpdatedAt.Set(fixed)).Where(suite.DocID.Eq(1)).SQL(); err == nil {
		t.Fatal("Set(updated_at) must still be refused on update")
	}
}
