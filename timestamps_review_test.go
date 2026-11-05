// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// unixSeconds is a named int64: not one of the supported timestamp types.
type unixSeconds int64

type namedStampRow struct {
	ID int64
	At unixSeconds
}

var namedStampMeta = &momo.Meta[namedStampRow]{
	Table: "named_stamps",
	Columns: []momo.ColumnMeta{
		{Name: "id", Flags: momo.FlagPK},
		{Name: "created_at", Flags: momo.FlagCreatedAt},
	},
	Ptr:      func(m *namedStampRow, i int) any { return []any{&m.ID, &m.At}[i] },
	Value:    func(m *namedStampRow, i int) any { return []any{m.ID, m.At}[i] },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: 1, UpdatedAt: -1,
	PK: []int{0},
}

func (*namedStampRow) MomoMeta() *momo.Meta[namedStampRow] { return namedStampMeta }

type nullStampRow struct {
	ID int64
	V  string
	At sql.NullTime
}

var nullStampV = momo.NewColumn[string]("null_stamps", "v", 0)

var nullStampMeta = &momo.Meta[nullStampRow]{
	Table: "null_stamps",
	Columns: []momo.ColumnMeta{
		{Name: "id", Flags: momo.FlagPK},
		{Name: "v"},
		{Name: "updated_at", Flags: momo.FlagUpdatedAt},
	},
	Ptr:      func(m *nullStampRow, i int) any { return []any{&m.ID, &m.V, &m.At}[i] },
	Value:    func(m *nullStampRow, i int) any { return []any{m.ID, m.V, m.At}[i] },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: 2,
	PK: []int{0},
}

func (*nullStampRow) MomoMeta() *momo.Meta[nullStampRow] { return nullStampMeta }

func TestAutoTimestampUnsupportedFieldTypeFails(t *testing.T) {
	db := momo.New(fakedriver.New(dialect.Postgres))
	cases := map[string]func() error{
		"named int64 insert Model": func() error {
			_, err := momo.Insert[namedStampRow](db).Model(&namedStampRow{ID: 1}).SQL()
			return err
		},
		"named int64 insert Values": func() error {
			_, err := momo.Insert[namedStampRow](db).Values().SQL()
			return err
		},
		"sql.NullTime update": func() error {
			_, err := momo.Update[nullStampRow](db).Set(nullStampV.Set("x")).Where(nullStampV.Eq("y")).SQL()
			return err
		},
	}
	for name, run := range cases {
		err := run()
		if !errors.Is(err, momo.ErrValidation) {
			t.Errorf("%s: err = %v, want ErrValidation", name, err)
			continue
		}
		if !strings.Contains(err.Error(), "unsupported field type") {
			t.Errorf("%s: error %q does not name the unsupported field type", name, err)
		}
	}
	if err := cases["named int64 insert Model"](); err == nil || !strings.Contains(err.Error(), "created_at") || !strings.Contains(err.Error(), "unixSeconds") {
		t.Errorf("error must name column and type, got %v", err)
	}
}

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
	ts, ok := c.Args[1].(time.Time)
	if !ok || ts.Location() != time.UTC || !ts.Equal(at) {
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
