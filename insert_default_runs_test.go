// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/suite"
)

// runInserts returns the INSERT statements the driver received, in order.
func runInserts(drv *fakedriver.Driver) []fakedriver.Call {
	var out []fakedriver.Call
	for _, c := range drv.Calls() {
		if strings.HasPrefix(c.SQL, "INSERT") {
			out = append(out, c)
		}
	}
	return out
}

// Catches: SQLite refusing a Models insert in which some rows leave a dbdefault
// column to the database, or rendering DEFAULT in VALUES (which SQLite rejects),
// or regrouping the rows so they are stored out of order. Rows that agree on
// their unset columns share one statement; the read-back of each run reaches
// the right models.
func TestSQLiteMixedModelsSplitIntoRuns(t *testing.T) {
	db, drv := dbdSetup(dialect.SQLite, [][]any{{"g1"}, {"g2"}})
	ms := []dbdRow{
		{ID: "kept", Title: "a", At: dbdStamp},
		{Title: "b", At: dbdStamp},
		{Title: "c", At: dbdStamp},
	}
	if _, err := momo.Insert[dbdRow](db).Models(ms).Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := runInserts(drv)
	want := []string{
		`INSERT INTO "dbd_rows" ("id", "title", "at") VALUES (?, ?, ?)`,
		`INSERT INTO "dbd_rows" ("title", "at") VALUES (?, ?), (?, ?) RETURNING "id"`,
	}
	if len(got) != len(want) {
		t.Fatalf("got %d INSERTs %v, want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i].SQL != want[i] {
			t.Errorf("statement %d = %q, want %q", i, got[i].SQL, want[i])
		}
		if strings.Contains(got[i].SQL, "DEFAULT") {
			t.Errorf("statement %d carries DEFAULT, which SQLite rejects: %q", i, got[i].SQL)
		}
	}
	if len(got[0].Args) != 3 || got[0].Args[0] != "kept" || len(got[1].Args) != 4 || got[1].Args[0] != "b" || got[1].Args[2] != "c" {
		t.Errorf("args = %v and %v", got[0].Args, got[1].Args)
	}
	if ms[0].ID != "kept" || ms[1].ID != "g1" || ms[2].ID != "g2" {
		t.Errorf("ids = %q %q %q, want kept g1 g2", ms[0].ID, ms[1].ID, ms[2].ID)
	}
}

// Catches: runs being merged across a row that differs (A B A must stay three
// statements, because regrouping the two A rows would reorder the inserts).
func TestSQLiteMixedModelsKeepRowOrder(t *testing.T) {
	db, drv := dbdSetup(dialect.SQLite, nil)
	ms := []dbdRow{
		{ID: "x", Title: "a", At: dbdStamp},
		{Title: "b", At: dbdStamp},
		{ID: "z", Title: "c", At: dbdStamp},
	}
	if _, err := momo.Insert[dbdRow](db).Models(ms).Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
	var titles []any
	for _, c := range runInserts(drv) {
		for _, a := range c.Args {
			if s, ok := a.(string); ok && (s == "a" || s == "b" || s == "c") {
				titles = append(titles, s)
			}
		}
	}
	if want := []any{"a", "b", "c"}; !reflect.DeepEqual(titles, want) {
		t.Fatalf("rows reached the database as %v, want %v", titles, want)
	}
	if n := len(runInserts(drv)); n != 3 {
		t.Fatalf("%d INSERTs, want 3", n)
	}
}

// Catches: PostgreSQL losing its single statement with DEFAULT inline (the
// split is for dialects that cannot put DEFAULT in VALUES) and MySQL being
// split instead of refusing what it cannot read back.
func TestMixedModelsOtherDialectsUnchanged(t *testing.T) {
	ms := []dbdRow{{Title: "a", At: dbdStamp}, {ID: "kept", Title: "b", At: dbdStamp}}
	db, drv := dbdSetup(dialect.Postgres, [][]any{{"g1"}, {"kept"}})
	if _, err := momo.Insert[dbdRow](db).Models(ms).Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := runInserts(drv)
	want := `INSERT INTO "dbd_rows" ("id", "title", "at") VALUES (DEFAULT, $1, $2), ($3, $4, $5) RETURNING "id"`
	if len(got) != 1 || got[0].SQL != want {
		t.Fatalf("PG = %v, want one statement %q", got, want)
	}
}

// Catches: SQLite refusing ValuesRows in which a column is SetDefault or
// omitted in some rows and given in others, or emitting DEFAULT. The three
// kinds of "default" row (SetDefault, omitted) fall in runs with the rows that
// agree on them.
func TestSQLiteMixedValuesRowsSplitIntoRuns(t *testing.T) {
	db, drv := dbdSetup(dialect.SQLite, nil)
	_, err := momo.Insert[suite.StatusDocument](db).ValuesRows(
		[]momo.Assignment{suite.DocTenantID.Set(1), suite.DocTitle.Set("n1"), suite.DocStatus.SetDefault()},
		[]momo.Assignment{suite.DocTenantID.Set(1), suite.DocTitle.Set("n2"), suite.DocStatus.Set("y")},
		[]momo.Assignment{suite.DocTenantID.Set(1), suite.DocTitle.Set("n3"), suite.DocStatus.Set("z")},
		[]momo.Assignment{suite.DocTenantID.Set(1), suite.DocTitle.Set("n4")},
	).Exec(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		`INSERT INTO "documents" ("tenant_id", "title") VALUES (?, ?)`,
		`INSERT INTO "documents" ("tenant_id", "title", "status") VALUES (?, ?, ?), (?, ?, ?)`,
		`INSERT INTO "documents" ("tenant_id", "title") VALUES (?, ?)`,
	}
	got := runInserts(drv)
	if len(got) != len(want) {
		t.Fatalf("got %d INSERTs %v, want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i].SQL != want[i] {
			t.Errorf("statement %d = %q, want %q", i, got[i].SQL, want[i])
		}
	}
	if got[1].Args[1] != "n2" || got[1].Args[2] != "y" || got[1].Args[4] != "n3" || got[1].Args[5] != "z" {
		t.Errorf("run 2 args = %v", got[1].Args)
	}
}

// Catches: the clock being read once per run, so automatic timestamps differ
// between the statements of one insert.
func TestSQLiteMixedValuesRowsShareOneTimestamp(t *testing.T) {
	db, drv := dbdSetup(dialect.SQLite, nil)
	_, err := momo.Insert[suite.Document](db).ValuesRows(
		[]momo.Assignment{suite.DocTenantID.Set(1), suite.DocTitle.Set("a"), suite.DocBody.Set("x")},
		[]momo.Assignment{suite.DocTenantID.Set(1), suite.DocTitle.Set("b")},
	).Exec(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := runInserts(drv)
	if len(got) != 2 {
		t.Fatalf("got %d INSERTs, want 2: %v", len(got), got)
	}
	last := func(c fakedriver.Call) (any, any) { return c.Args[len(c.Args)-2], c.Args[len(c.Args)-1] }
	c1, u1 := last(got[0])
	c2, u2 := last(got[1])
	if c1 != c2 || u1 != u2 {
		t.Fatalf("timestamps differ between runs: %v %v vs %v %v", c1, u1, c2, u2)
	}
}
