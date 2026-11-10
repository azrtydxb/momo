// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/suite"
)

func clockAt(t time.Time) momo.Option { return momo.WithClock(func() time.Time { return t }) }

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
