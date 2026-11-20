// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/compositeop"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/types"
)

// dcAddress is the Go struct of the composite type dc_address.
type dcAddress struct {
	Street string
	Zip    *string
	Floor  int32
}

// dcRow is a row of dc_customers: a column of a domain over text, and two
// columns of a composite type, one of them nullable.
type dcRow struct {
	ID    int64
	Email string
	Home  types.Composite[dcAddress]
	Work  *types.Composite[dcAddress]
}

var dcRowMeta = momo.Meta[dcRow]{
	Columns: []momo.ColumnMeta{{Name: "id"}, {Name: "email"}, {Name: "home"}, {Name: "work"}},
	Ptr: func(m *dcRow, i int) any {
		return [...]any{&m.ID, &m.Email, &m.Home, &m.Work}[i]
	},
	Value: func(m *dcRow, i int) any {
		return [...]any{m.ID, m.Email, m.Home, m.Work}[i]
	},
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
}

func (*dcRow) MomoMeta() *momo.Meta[dcRow] { return &dcRowMeta }

var (
	dcID    = momo.NewColumn[int64]("dc_customers", "id", momo.FlagPK)
	dcEmail = momo.NewColumn[string]("dc_customers", "email", 0)
	dcHome  = momo.NewColumn[types.Composite[dcAddress]]("dc_customers", "home", 0)
	dcWork  = momo.NewColumn[types.Composite[dcAddress]]("dc_customers", "work", 0)
)

var dcDDL = []string{
	`CREATE DOMAIN dc_email AS text NOT NULL CONSTRAINT dc_email_shape CHECK (VALUE LIKE '%@%')`,
	`CREATE TYPE dc_address AS (street text, zip text, floor integer)`,
	`CREATE TABLE dc_customers (id bigint PRIMARY KEY, email dc_email, home dc_address NOT NULL, work dc_address)`,
}

func dcSetup(t *testing.T) *momo.DB {
	t.Helper()
	db := testopen.Postgres(t)
	for _, sql := range dcDDL {
		if _, err := momo.Exec(context.Background(), db, sql); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	return db
}

func dcStr(s string) *string { return &s }

func dcRows(t *testing.T, db *momo.DB, where momo.Predicate) []dcRow {
	t.Helper()
	q := momo.Select[dcRow](db).From(momo.TableRef{Name: "dc_customers"}).Columns(dcID, dcEmail, dcHome, dcWork).OrderBy(dcID.Asc())
	if where != nil {
		q = q.Where(where)
	}
	rows, err := q.All(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

// Catches: a composite value that does not survive the trip through its
// PostgreSQL column, for the shapes of record text that are easy to get wrong:
// a quote, a comma, a backslash and a parenthesis in a field, an empty string
// told from NULL, a NULL attribute, a NULL column, and a negative number; and a
// column of a domain that does not scan as its base type.
func TestCompositeAndDomainRoundTripLive(t *testing.T) {
	db := dcSetup(t)
	ctx := context.Background()
	rows := []dcRow{
		{ID: 1, Email: "a@b.c", Home: types.Composite[dcAddress]{V: dcAddress{Street: `Main "St", 5`, Zip: dcStr("1000"), Floor: 3}}},
		{ID: 2, Email: "x@y.z", Home: types.Composite[dcAddress]{V: dcAddress{}}, Work: &types.Composite[dcAddress]{V: dcAddress{Street: `back\slash (x)`, Zip: dcStr(""), Floor: -1}}},
	}
	for _, r := range rows {
		if _, err := momo.Exec(ctx, db, "INSERT INTO dc_customers (id, email, home, work) VALUES (?, ?, ?, ?)", r.ID, r.Email, r.Home, r.Work); err != nil {
			t.Fatalf("insert %d: %v", r.ID, err)
		}
	}
	got := dcRows(t, db, nil)
	if !reflect.DeepEqual(got, rows) {
		t.Fatalf("rows read back differ:\n got %+v\nwant %+v", got, rows)
	}
	// The empty string and NULL stay apart inside the record.
	if got[0].Home.V.Zip == nil || got[1].Home.V.Zip != nil || got[1].Work.V.Zip == nil || *got[1].Work.V.Zip != "" {
		t.Errorf("zip values: %+v", got)
	}
}

// Catches: equality on a whole composite value that compares the record text
// instead of the record, a NULL column that is not found by IsNull, and an
// UPDATE of a composite column that does not write the new value.
func TestCompositeEqualityAndUpdateLive(t *testing.T) {
	db := dcSetup(t)
	ctx := context.Background()
	home := types.Composite[dcAddress]{V: dcAddress{Street: "Main St", Zip: dcStr("1000"), Floor: 3}}
	other := types.Composite[dcAddress]{V: dcAddress{Street: "Side St", Floor: 3}}
	for _, r := range []dcRow{{ID: 1, Email: "a@b.c", Home: home}, {ID: 2, Email: "x@y.z", Home: other, Work: &home}} {
		if _, err := momo.Exec(ctx, db, "INSERT INTO dc_customers (id, email, home, work) VALUES (?, ?, ?, ?)", r.ID, r.Email, r.Home, r.Work); err != nil {
			t.Fatal(err)
		}
	}
	ids := func(p momo.Predicate) []int64 {
		out := []int64{}
		for _, r := range dcRows(t, db, p) {
			out = append(out, r.ID)
		}
		return out
	}
	for name, c := range map[string]struct {
		pred momo.Predicate
		want []int64
	}{
		"home equals":      {compositeop.Equals(dcHome, "dc_address", home), []int64{1}},
		"other equals":     {compositeop.Equals(dcHome, "dc_address", other), []int64{2}},
		"home differs":     {compositeop.NotEquals(dcHome, "dc_address", home), []int64{2}},
		"work equals":      {compositeop.Equals(dcWork, "dc_address", home), []int64{2}},
		"work is null":     {dcWork.IsNull(), []int64{1}},
		"a missing value":  {compositeop.Equals(dcHome, "dc_address", types.Composite[dcAddress]{V: dcAddress{Street: "Main St", Floor: 3}}), []int64{}},
		"domain and value": {momo.And(dcEmail.Eq("x@y.z"), compositeop.Equals(dcHome, "dc_address", other)), []int64{2}},
	} {
		if got := ids(c.pred); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: ids = %v, want %v", name, got, c.want)
		}
	}

	moved := types.Composite[dcAddress]{V: dcAddress{Street: "New Rd", Zip: dcStr("2000"), Floor: 9}}
	n, err := momo.Exec(ctx, db, "UPDATE dc_customers SET home = ?, work = ? WHERE id = ?", moved, nil, int64(2))
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("UPDATE changed %d rows", n)
	}
	if got := ids(compositeop.Equals(dcHome, "dc_address", moved)); !reflect.DeepEqual(got, []int64{2}) {
		t.Errorf("after the update, home equals moved: ids = %v", got)
	}
	if got := ids(dcWork.IsNull()); !reflect.DeepEqual(got, []int64{1, 2}) {
		t.Errorf("after the update, work is null: ids = %v", got)
	}
}

// Catches: a domain whose rules are not enforced by the database momo created
// it in, which would leave the model promising a check that does nothing.
func TestDomainRulesEnforcedLive(t *testing.T) {
	db := dcSetup(t)
	ctx := context.Background()
	home := types.Composite[dcAddress]{V: dcAddress{Street: "Main St"}}
	if _, err := momo.Exec(ctx, db, "INSERT INTO dc_customers (id, email, home) VALUES (?, ?, ?)", int64(1), "no-at-sign", home); err == nil {
		t.Error("an email without @ broke the domain check and was accepted")
	}
	if _, err := momo.Exec(ctx, db, "INSERT INTO dc_customers (id, home) VALUES (?, ?)", int64(2), home); err == nil {
		t.Error("a NULL email broke the domain NOT NULL and was accepted")
	}
	if rows := dcRows(t, db, nil); len(rows) != 0 {
		t.Errorf("rows after refused inserts: %+v", rows)
	}
}
