// SPDX-License-Identifier: Apache-2.0

//go:build integration && cgo && !windows

package sqlite_test

import (
	"context"
	"slices"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/migrate"
	msqlite "github.com/azrtydxb/momo/migrate/sqlite"
	"github.com/azrtydxb/momo/schema"

	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
)

func TestSQLiteInspectRoundTrip(t *testing.T) {
	db := testopen.SQLite(t)
	ctx := context.Background()
	if _, err := db.Driver().Exec(ctx, `CREATE TABLE docs (id INTEGER PRIMARY KEY, title TEXT NOT NULL DEFAULT '', status TEXT CHECK ("status" IN ('a','b')), tenant INTEGER REFERENCES tenants(id) ON DELETE CASCADE)`, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Driver().Exec(ctx, `CREATE INDEX docs_title ON docs (lower(title)) WHERE status = 'a'`, nil); err != nil {
		t.Fatal(err)
	}
	c := pinned(t, db)
	got, err := msqlite.New().Inspect(ctx, c, migrate.InspectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	tbl, ok := got.Table("docs")
	if !ok {
		t.Fatal("docs not inspected")
	}
	col, ok := tbl.Column("title")
	if !ok {
		t.Fatalf("column %q missing", "title")
	}
	if col.Nullable || col.Default != "''" {
		t.Fatalf("title: %+v", col)
	}
	if len(tbl.ForeignKeys) != 1 || tbl.ForeignKeys[0].OnDelete != "CASCADE" {
		t.Fatalf("fk actions lost: %+v", tbl.ForeignKeys)
	}
	if len(tbl.Indexes) != 1 || tbl.Indexes[0].Where == "" || tbl.Indexes[0].Parts[0].Expr == "" {
		t.Fatalf("expression or partial index lost: %+v", tbl.Indexes)
	}
	if !schema.Equal(got, schema.Normalized(got)) {
		t.Fatal("inspection output must be stable under normalization")
	}
	if got.Dialect != dialect.SQLite {
		t.Fatalf("dialect = %q", got.Dialect)
	}
}

var inspectDDL = []string{
	`CREATE TABLE parents (id INTEGER PRIMARY KEY, code TEXT NOT NULL COLLATE NOCASE, CONSTRAINT parents_code_key UNIQUE (code))`,
	`CREATE TABLE kids (
  a INTEGER NOT NULL, b INTEGER NOT NULL, parent_id INTEGER,
  label TEXT NOT NULL DEFAULT 'x',
  mood TEXT CHECK (mood IN ('happy', 'sad')),
  total INTEGER GENERATED ALWAYS AS (a + b) STORED,
  CONSTRAINT kids_pk PRIMARY KEY (a, b),
  CONSTRAINT kids_label_len CHECK (length(label) > 0),
  CONSTRAINT kids_parent FOREIGN KEY (parent_id) REFERENCES parents ON DELETE SET NULL ON UPDATE CASCADE,
  UNIQUE (label, parent_id)
)`,
	`CREATE UNIQUE INDEX kids_total ON kids (total DESC)`,
	`CREATE TABLE wr (k TEXT PRIMARY KEY, v BLOB) WITHOUT ROWID`,
	`CREATE VIEW kid_labels AS SELECT label FROM kids`,
	`CREATE TRIGGER kids_ai AFTER INSERT ON kids BEGIN SELECT 1; END`,
	`CREATE TABLE momo_migrations (version TEXT PRIMARY KEY)`,
	`CREATE TABLE momo_migration_lock (id INTEGER PRIMARY KEY, holder TEXT)`,
}

func TestSQLiteInspectDetails(t *testing.T) {
	db := testopen.SQLite(t)
	ctx := context.Background()
	c := pinned(t, db)
	for _, stmt := range inspectDDL {
		mustExec(t, c, stmt)
	}
	got, err := msqlite.New().Inspect(ctx, c, migrate.InspectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tb := range got.Tables {
		names = append(names, tb.Name)
	}
	if !slices.Equal(names, []string{"kids", "parents", "wr"}) {
		t.Fatalf("tables %v: the bookkeeping tables must not be reported", names)
	}

	parents, _ := got.Table("parents")
	if id, _ := parents.Column("id"); id.Identity != "by_default" {
		t.Errorf("an INTEGER PRIMARY KEY is the rowid and must be reported as by_default: %+v", id)
	}
	if code, _ := parents.Column("code"); code.Collate != "NOCASE" || code.Nullable {
		t.Errorf("parents.code: %+v", code)
	}
	if len(parents.Uniques) != 1 || parents.Uniques[0].Name != "parents_code_key" {
		t.Errorf("a named UNIQUE keeps its name: %+v", parents.Uniques)
	}

	kids, _ := got.Table("kids")
	if !slices.Equal(kids.PrimaryKey, []string{"a", "b"}) {
		t.Errorf("composite primary key: %v", kids.PrimaryKey)
	}
	if a, _ := kids.Column("a"); a.Identity != "" {
		t.Errorf("a composite key column is not a rowid alias: %+v", a)
	}
	total, _ := kids.Column("total")
	if schema.NormalizeExpr(dialect.SQLite, total.Generated) != "a + b" {
		t.Errorf("generated expression: %+v", total)
	}
	mood, _ := kids.Column("mood")
	if mood.Enum != "kids_mood" || !mood.Nullable {
		t.Errorf("mood must be an enum column: %+v", mood)
	}
	if !slices.Equal(got.Enums[0].Values, []string{"happy", "sad"}) || got.Enums[0].Name != "kids_mood" || len(got.Enums) != 1 {
		t.Errorf("enums: %+v", got.Enums)
	}
	if len(kids.Checks) != 1 || kids.Checks[0].Name != "kids_label_len" {
		t.Errorf("the enum CHECK must not also be a Check: %+v", kids.Checks)
	}
	if len(kids.Uniques) != 1 || kids.Uniques[0].Name != "kids_label_parent_id_key" || !slices.Equal(kids.Uniques[0].Columns, []string{"label", "parent_id"}) {
		t.Errorf("an unnamed UNIQUE gets a derived name: %+v", kids.Uniques)
	}
	if len(kids.ForeignKeys) != 1 {
		t.Fatalf("foreign keys: %+v", kids.ForeignKeys)
	}
	fk := kids.ForeignKeys[0]
	if fk.Name != "kids_parent" || fk.RefTable != "parents" || !slices.Equal(fk.RefColumns, []string{"id"}) || fk.OnDelete != "SET NULL" || fk.OnUpdate != "CASCADE" {
		t.Errorf("a REFERENCES with no column list targets the parent's primary key: %+v", fk)
	}
	if len(kids.Indexes) != 1 || kids.Indexes[0].Name != "kids_total" || !kids.Indexes[0].Unique || !kids.Indexes[0].Parts[0].Desc || kids.Indexes[0].Parts[0].Column != "total" {
		t.Errorf("unique descending index: %+v", kids.Indexes)
	}

	wr, _ := got.Table("wr")
	if !wr.WithoutRowID {
		t.Error("WITHOUT ROWID not reported")
	}
	if k, _ := wr.Column("k"); k.Identity != "" {
		t.Errorf("a WITHOUT ROWID table has no rowid alias: %+v", k)
	}

	if len(got.Views) != 1 || got.Views[0].Name != "kid_labels" || got.Views[0].Query != "SELECT label FROM kids" || !slices.Equal(got.Views[0].Depends, []string{"kids"}) {
		t.Errorf("views: %+v", got.Views)
	}
	if len(got.Triggers) != 1 || got.Triggers[0].Name != "kids_ai" || got.Triggers[0].Table != "kids" ||
		got.Triggers[0].Definition != "CREATE TRIGGER kids_ai AFTER INSERT ON kids BEGIN SELECT 1; END" {
		t.Errorf("triggers: %+v", got.Triggers)
	}
	if len(got.Extensions)+len(got.Functions)+len(got.Policies)+len(got.Schemas) != 0 {
		t.Errorf("SQLite has no extensions, functions, policies or schemas to report: %+v", got)
	}
}

// TestSQLiteInspectRowidAliasIsNotNull checks that an INTEGER PRIMARY KEY, the
// rowid, is reported NOT NULL although the pragma says nullable for one that is
// not declared so, and that the columns that are not a rowid alias keep what
// the database says.
//
// Catches: the rowid alias reported nullable, so a model that declares it NOT
// NULL differs from its own database; or the fix reaching columns that are not
// the rowid (a DESC key, a WITHOUT ROWID key, a composite key).
func TestSQLiteInspectRowidAliasIsNotNull(t *testing.T) {
	db := testopen.SQLite(t)
	c := pinned(t, db)
	for _, stmt := range []string{
		`CREATE TABLE alias (id INTEGER PRIMARY KEY, v TEXT)`,
		`CREATE TABLE descending (id INTEGER PRIMARY KEY DESC, v TEXT)`,
		`CREATE TABLE norowid (id INTEGER PRIMARY KEY, v TEXT) WITHOUT ROWID`,
		`CREATE TABLE composite (a INTEGER, b INTEGER, PRIMARY KEY (a, b))`,
		`CREATE TABLE textkey (k TEXT PRIMARY KEY, v TEXT)`,
	} {
		mustExec(t, c, stmt)
	}
	got, err := msqlite.New().Inspect(context.Background(), c, migrate.InspectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		table, col string
		nullable   bool
		identity   string
	}{
		{"alias", "id", false, "by_default"},
		{"descending", "id", true, ""},
		{"norowid", "id", false, ""}, // WITHOUT ROWID: SQLite reports NOT NULL for its key columns
		{"composite", "a", true, ""},
		{"composite", "b", true, ""},
		{"textkey", "k", true, ""},
	} {
		tbl, ok := got.Table(tc.table)
		if !ok {
			t.Fatalf("%s not inspected", tc.table)
		}
		col, _ := tbl.Column(tc.col)
		if col.Nullable != tc.nullable || col.Identity != tc.identity {
			t.Errorf("%s.%s: nullable=%v identity=%q, want nullable=%v identity=%q", tc.table, tc.col, col.Nullable, col.Identity, tc.nullable, tc.identity)
		}
	}
}
