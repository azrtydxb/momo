// SPDX-License-Identifier: Apache-2.0

//go:build integration && cgo && !windows

package sqlite_test

import (
	"context"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/migrate"
	msqlite "github.com/azrtydxb/momo/migrate/sqlite"
	"github.com/azrtydxb/momo/schema"

	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
)

const touchTrigger = "CREATE TRIGGER docs_touch AFTER UPDATE ON docs BEGIN UPDATE docs SET score = score WHERE id = NEW.id; END"

// roundTripSchema is a schema with every kind of object SQLite can hold, in the
// spelling a model gives it. v2 changes it in ways that force a rebuild.
func roundTripSchema(v2 bool) *schema.Schema {
	statusValues := []string{"a", "b", "c"}
	titleDefault := "''"
	extraCheck := []schema.Check{{Name: "docs_title_len", Expr: "length(title) > 0"}}
	if v2 {
		statusValues = append(statusValues, "d")
		titleDefault = "'untitled'"
		extraCheck = append(extraCheck, schema.Check{Name: "docs_score_pos", Expr: "score >= 0"})
	}
	s := &schema.Schema{
		Dialect: dialect.SQLite,
		Enums:   []schema.Enum{{Name: "docs_status", Values: statusValues}},
		Tables: []schema.Table{
			{
				Name: "tenants",
				Columns: []schema.Column{
					{Name: "id", Type: "INTEGER", Identity: "by_default"},
					{Name: "name", Type: "TEXT"},
				},
				PrimaryKey: []string{"id"},
				Uniques:    []schema.Unique{{Name: "tenants_name_key", Columns: []string{"name"}}},
			},
			{
				Name: "docs",
				Columns: []schema.Column{
					{Name: "id", Type: "INTEGER"},
					{Name: "title", Type: "TEXT", Default: titleDefault},
					{Name: "status", Type: "TEXT", Nullable: true, Enum: "docs_status"},
					{Name: "tenant_id", Type: "INTEGER", Nullable: true},
					{Name: "score", Type: "REAL", Nullable: true, Default: "0.5"},
					{Name: "created", Type: "TEXT", Default: "datetime('now')"},
					{Name: "slug", Type: "TEXT", Nullable: true, Generated: "lower(title)"},
				},
				PrimaryKey: []string{"id"},
				Checks:     extraCheck,
				ForeignKeys: []schema.ForeignKey{
					{Name: "docs_tenant_id_fkey", Columns: []string{"tenant_id"}, RefTable: "tenants", RefColumns: []string{"id"}, OnDelete: "CASCADE"},
				},
				Indexes: []schema.Index{
					{Name: "docs_title", Parts: []schema.IndexPart{{Expr: "lower(title)"}}, Where: "status = 'a'"},
					{Name: "docs_tenant_title", Unique: true, Parts: []schema.IndexPart{{Column: "tenant_id"}, {Column: "title", Desc: true}}},
				},
			},
		},
		Views:    []schema.View{{Name: "open_docs", Query: "SELECT id, title FROM docs WHERE status = 'a'", Depends: []string{"docs"}}},
		Triggers: []schema.Trigger{{Name: "docs_touch", Table: "docs", Definition: touchTrigger}},
	}
	return s
}

// render plans and renders the change from one schema to the next the way
// Engine.Diff does.
func render(t *testing.T, from, to *schema.Schema) []migrate.Statement {
	t.Helper()
	changes, _, err := migrate.Diff(from, to, migrate.DiffOptions{AllowDestructive: true})
	if err != nil {
		t.Fatal(err)
	}
	ordered, err := migrate.OrderWithViews(changes, from.Views, to.Views)
	if err != nil {
		t.Fatal(err)
	}
	stmts, err := msqlite.New().RenderSchema(from, to, ordered)
	if err != nil {
		t.Fatal(err)
	}
	return stmts
}

// apply runs stmts the way Apply does: foreign keys off outside the
// transaction when the plan ends in a rebuild's foreign_key_check.
func apply(t *testing.T, c momo.PinnedConn, stmts []migrate.Statement) {
	t.Helper()
	ctx := context.Background()
	reset, err := msqlite.New().Setup(ctx, c, migrate.SessionOptions{ForeignKeysOff: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate.RunStatements(ctx, c, stmts); err != nil {
		_ = reset()
		t.Fatalf("apply failed: %v\nstatements:\n%s", err, dump(stmts))
	}
	if err := reset(); err != nil {
		t.Fatal(err)
	}
}

func dump(stmts []migrate.Statement) string {
	var b strings.Builder
	for _, s := range stmts {
		b.WriteString(s.SQL + ";\n")
	}
	return b.String()
}

// assertNoDiff fails with the changes Diff still finds between want and the
// inspected database.
func assertNoDiff(t *testing.T, c momo.PinnedConn, want *schema.Schema) {
	t.Helper()
	got, err := msqlite.New().Inspect(context.Background(), c, migrate.InspectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	changes, _, err := migrate.Diff(want, got, migrate.DiffOptions{AllowDestructive: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 {
		var kinds []string
		for _, ch := range changes {
			kinds = append(kinds, string(ch.Kind)+" "+ch.Table+"."+ch.Name)
		}
		t.Fatalf("render, apply, inspect and diff must find nothing; found %v\n%s", kinds, migrateDiff(got, want))
	}
	if !schema.Equal(want, got) {
		t.Fatalf("Diff found nothing but the schemas are not Equal\n%s", migrateDiff(got, want))
	}
}

func migrateDiff(got, want *schema.Schema) string {
	return "got:\n" + jsonOf(schema.Normalized(got)) + "\nwant:\n" + jsonOf(schema.Normalized(want))
}

func jsonOf(s *schema.Schema) string {
	var b strings.Builder
	_ = s.Write(&b)
	return b.String()
}

func TestSQLiteRenderApplyInspectDiffIsZero(t *testing.T) {
	db := testopen.SQLite(t)
	c := pinned(t, db)
	v1 := roundTripSchema(false)
	apply(t, c, render(t, &schema.Schema{Dialect: dialect.SQLite}, v1))
	assertNoDiff(t, c, v1)
}

// TestSQLiteRebuildKeepsDataAndObjects moves a populated database from v1 to v2,
// which changes a default, adds a check and widens an enum: three changes that
// ALTER TABLE cannot make. The rows, the index, the trigger and the view must
// survive, and the result must inspect equal to v2.
func TestSQLiteRebuildKeepsDataAndObjects(t *testing.T) {
	db := testopen.SQLite(t)
	c := pinned(t, db)
	v1, v2 := roundTripSchema(false), roundTripSchema(true)
	apply(t, c, render(t, &schema.Schema{Dialect: dialect.SQLite}, v1))

	mustExec(t, c, `INSERT INTO tenants (id, name) VALUES (1, 'acme'), (2, 'globex')`)
	mustExec(t, c, `INSERT INTO docs (title, status, tenant_id, score) VALUES ('One', 'a', 1, 1.5), ('Two', 'b', 2, 2.5), ('Three', NULL, NULL, 0)`)

	stmts := render(t, v1, v2)
	if !strings.Contains(dump(stmts), `CREATE TABLE "new_docs"`) {
		t.Fatalf("the change must rebuild docs:\n%s", dump(stmts))
	}
	apply(t, c, stmts)

	var n int
	scanOne(t, c, `SELECT count(*) FROM docs`, &n)
	if n != 3 {
		t.Fatalf("rows lost: %d", n)
	}
	var titles string
	scanOne(t, c, `SELECT group_concat(title, ',') FROM (SELECT title FROM docs ORDER BY id)`, &titles)
	if titles != "One,Two,Three" {
		t.Fatalf("row content changed: %q", titles)
	}
	var slug string
	scanOne(t, c, `SELECT slug FROM docs WHERE id = 1`, &slug)
	if slug != "one" {
		t.Fatalf("the generated column must be recomputed, got %q", slug)
	}
	for _, o := range [][2]string{{"index", "docs_title"}, {"index", "docs_tenant_title"}, {"trigger", "docs_touch"}, {"view", "open_docs"}} {
		var count int
		scanOne(t, c, `SELECT count(*) FROM sqlite_schema WHERE type='`+o[0]+`' AND name='`+o[1]+`'`, &count)
		if count != 1 {
			t.Errorf("%s %s missing after the rebuild", o[0], o[1])
		}
	}
	var viewRows int
	scanOne(t, c, `SELECT count(*) FROM open_docs`, &viewRows)
	if viewRows != 1 {
		t.Errorf("the recreated view reads %d rows, want 1", viewRows)
	}
	var violations int
	scanOne(t, c, `SELECT count(*) FROM pragma_foreign_key_check`, &violations)
	if violations != 0 {
		t.Errorf("foreign_key_check returned %d rows", violations)
	}
	var fk int
	scanOne(t, c, `PRAGMA foreign_keys`, &fk)
	if fk != 1 {
		t.Errorf("foreign keys must be back on, got %d", fk)
	}
	if _, err := c.Exec(context.Background(), `INSERT INTO docs (title, status) VALUES ('Bad', 'z')`, nil); err == nil {
		t.Error("the widened enum CHECK must still reject an unknown value")
	}
	if _, err := c.Exec(context.Background(), `INSERT INTO docs (title, status) VALUES ('Fine', 'd')`, nil); err != nil {
		t.Errorf("the new enum value must be accepted: %v", err)
	}
	assertNoDiff(t, c, v2)
}

// TestSQLiteNativeAddColumnNeedsNoRebuild checks that a nullable column is
// added by ALTER TABLE ADD COLUMN, without touching the table.
func TestSQLiteNativeAddColumnNeedsNoRebuild(t *testing.T) {
	db := testopen.SQLite(t)
	c := pinned(t, db)
	v1 := roundTripSchema(false)
	apply(t, c, render(t, &schema.Schema{Dialect: dialect.SQLite}, v1))
	v2 := roundTripSchema(false)
	docs := &v2.Tables[1]
	docs.Columns = append(docs.Columns, schema.Column{Name: "note", Type: "TEXT", Nullable: true})
	stmts := render(t, v1, v2)
	if len(stmts) != 1 || stmts[0].SQL != `ALTER TABLE "docs" ADD COLUMN "note" TEXT` {
		t.Fatalf("want one native ADD COLUMN, got:\n%s", dump(stmts))
	}
	apply(t, c, stmts)
	assertNoDiff(t, c, v2)
}

// TestSQLiteDropAndRenameNative checks the native drop of an unused column and
// the rename of a column and a table.
func TestSQLiteDropAndRenameNative(t *testing.T) {
	db := testopen.SQLite(t)
	c := pinned(t, db)
	v1 := &schema.Schema{Dialect: dialect.SQLite, Tables: []schema.Table{{
		Name:       "t",
		Columns:    []schema.Column{{Name: "id", Type: "INTEGER"}, {Name: "a", Type: "TEXT", Nullable: true}, {Name: "b", Type: "TEXT", Nullable: true}},
		PrimaryKey: []string{"id"},
	}}}
	apply(t, c, render(t, &schema.Schema{Dialect: dialect.SQLite}, v1))
	mustExec(t, c, `INSERT INTO t (a, b) VALUES ('x', 'y')`)

	v2 := &schema.Schema{Dialect: dialect.SQLite, Tables: []schema.Table{{
		Name:       "t2",
		Columns:    []schema.Column{{Name: "id", Type: "INTEGER"}, {Name: "a2", Type: "TEXT", Nullable: true}},
		PrimaryKey: []string{"id"},
	}}}
	changes, _, err := migrate.Diff(v1, v2, migrate.DiffOptions{
		AllowDestructive: true,
		RenameTables:     map[string]string{"t": "t2"},
		RenameColumns:    map[string]string{"t2.a": "a2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	ordered, err := migrate.OrderWithViews(changes, v1.Views, v2.Views)
	if err != nil {
		t.Fatal(err)
	}
	stmts, err := msqlite.New().RenderSchema(v1, v2, ordered)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range stmts {
		if strings.Contains(s.SQL, "new_") {
			t.Fatalf("rename and an unused drop must not rebuild:\n%s", dump(stmts))
		}
	}
	apply(t, c, stmts)
	var a2 string
	scanOne(t, c, `SELECT a2 FROM t2`, &a2)
	if a2 != "x" {
		t.Fatalf("renamed column lost its data: %q", a2)
	}
	assertNoDiff(t, c, v2)
}

// TestSQLiteStrictAutoincrementRoundTrip creates a STRICT AUTOINCREMENT table,
// inspects it back, and rebuilds it: the options must survive all three.
func TestSQLiteStrictAutoincrementRoundTrip(t *testing.T) {
	db := testopen.SQLite(t)
	c := pinned(t, db)
	mk := func(def string) *schema.Schema {
		return &schema.Schema{Dialect: dialect.SQLite, Tables: []schema.Table{{
			Name:          "events",
			Columns:       []schema.Column{{Name: "id", Type: "INTEGER"}, {Name: "body", Type: "TEXT", Nullable: true, Default: def}},
			PrimaryKey:    []string{"id"},
			Autoincrement: true,
			Strict:        true,
		}}}
	}
	v1, v2 := mk(""), mk("'x'")
	apply(t, c, render(t, &schema.Schema{Dialect: dialect.SQLite}, v1))
	assertNoDiff(t, c, v1)
	got, err := msqlite.New().Inspect(context.Background(), c, migrate.InspectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if tbl, _ := got.Table("events"); !tbl.Strict || !tbl.Autoincrement {
		t.Fatalf("inspection lost STRICT or AUTOINCREMENT: %+v", tbl)
	}

	mustExec(t, c, `INSERT INTO events (body) VALUES ('a'), ('b')`)
	apply(t, c, render(t, v1, v2))
	assertNoDiff(t, c, v2)
	var text string
	scanOne(t, c, `SELECT sql FROM sqlite_schema WHERE name = 'events'`, &text)
	if !strings.Contains(text, "AUTOINCREMENT") || !strings.HasSuffix(strings.TrimSpace(text), "STRICT") {
		t.Fatalf("the rebuild dropped an option: %s", text)
	}
	var n int
	scanOne(t, c, `SELECT count(*) FROM events`, &n)
	if n != 2 {
		t.Fatalf("rows lost: %d", n)
	}
}

// TestSQLiteInspectDescKeyIsNotRowid checks that INTEGER PRIMARY KEY DESC, which
// is an ordinary index and not the rowid, is not reported as an identity.
func TestSQLiteInspectDescKeyIsNotRowid(t *testing.T) {
	db := testopen.SQLite(t)
	c := pinned(t, db)
	mustExec(t, c, `CREATE TABLE d (id INTEGER PRIMARY KEY DESC, v TEXT)`)
	mustExec(t, c, `CREATE TABLE r (id INTEGER PRIMARY KEY, v TEXT)`)
	got, err := msqlite.New().Inspect(context.Background(), c, migrate.InspectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	d, _ := got.Table("d")
	r, _ := got.Table("r")
	if col, _ := d.Column("id"); col.Identity != "" {
		t.Errorf("PRIMARY KEY DESC is not a rowid alias: %+v", col)
	}
	if col, _ := r.Column("id"); col.Identity != "by_default" {
		t.Errorf("PRIMARY KEY is a rowid alias: %+v", col)
	}
}
