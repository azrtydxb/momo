// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

func TestSQLiteAlterDecision(t *testing.T) {
	nullDefault := &schema.Column{Name: "n", Type: "TEXT", Nullable: true}
	notNullNoDefault := &schema.Column{Name: "n", Type: "TEXT"}
	notNullDefault := &schema.Column{Name: "n", Type: "TEXT", Default: "''"}
	cases := []struct {
		name string
		ch   migrate.Change
		want bool // true means Rebuild
	}{
		{"nullable add is native", migrate.Change{Kind: "add_column", Table: "t", After: nullDefault}, false},
		{"not null without default must rebuild", migrate.Change{Kind: "add_column", Table: "t", After: notNullNoDefault}, true},
		{"not null with default is native", migrate.Change{Kind: "add_column", Table: "t", After: notNullDefault}, false},
		{"alter column rebuilds", migrate.Change{Kind: "alter_column", Table: "t", Before: nullDefault, After: notNullDefault}, true},
		{"add fk rebuilds", migrate.Change{Kind: "add_fk", Table: "t"}, true},
		{"rename column is native", migrate.Change{Kind: "rename_column", Table: "t", Name: "b"}, false},
	}
	for _, c := range cases {
		stmts, err := Dialect{}.Render([]migrate.Change{c.ch})
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got := len(stmts) > 0 && stmts[0].Hint == "rebuild"; got != c.want {
			t.Errorf("%s: rebuild=%v want %v", c.name, got, c.want)
		}
	}
}

func TestSQLiteUnsupportedKindsError(t *testing.T) {
	_, err := Dialect{}.Render([]migrate.Change{{Kind: "create_enum", Name: "e"}})
	if err == nil {
		t.Fatal("create_enum must not be silently dropped on SQLite")
	}
}

// TestSQLiteUnsupportedKindsAreTyped fails if a kind SQLite lacks is dropped or
// returned as an untyped error: each one must wrap momo.ErrUnsupported.
func TestSQLiteUnsupportedKindsAreTyped(t *testing.T) {
	for _, k := range []migrate.ChangeKind{
		migrate.CreateSchema, migrate.CreateExtension, migrate.CreateEnum, migrate.AlterEnumAddValue, migrate.DropEnum,
		migrate.CreatePolicy, migrate.DropPolicy, migrate.SetRLS,
		migrate.CreateFunction, migrate.ReplaceFunction, migrate.DropFunction,
	} {
		_, err := Dialect{}.Render([]migrate.Change{{Kind: k, Name: "x"}})
		if !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("%s: want an error wrapping momo.ErrUnsupported, got %v", k, err)
		}
	}
}

// TestSQLiteRenderNativeStatements pins the text of every native statement.
func TestSQLiteRenderNativeStatements(t *testing.T) {
	col := &schema.Column{Name: "n", Type: "INTEGER", Nullable: true, Default: "7"}
	plan := []migrate.Change{
		{Kind: migrate.RenameTable, Table: "old", Name: "new"},
		{Kind: migrate.RenameColumn, Table: "new", Name: "b", Before: &schema.Column{Name: "a"}, After: &schema.Column{Name: "b"}},
		{Kind: migrate.AddColumn, Table: "new", Name: "n", After: col},
		{Kind: migrate.CreateIndex, Table: "new", Name: "ix", After: &schema.Index{Name: "ix", Unique: true, Parts: []schema.IndexPart{{Column: "b", Desc: true}, {Expr: "lower(n)"}}, Where: "n > 1"}},
		{Kind: migrate.DropIndex, Table: "new", Name: "gone", IfExists: true},
		{Kind: migrate.CreateView, Name: "v", After: &schema.View{Name: "v", Query: "SELECT b FROM new;"}},
		{Kind: migrate.DropView, Name: "w"},
		{Kind: migrate.CreateTrigger, Table: "new", Name: "tr", After: &schema.Trigger{Name: "tr", Table: "new", Definition: "CREATE TRIGGER tr AFTER INSERT ON new BEGIN SELECT 1; END;"}},
		{Kind: migrate.DropTrigger, Table: "new", Name: "tq"},
		{Kind: migrate.DropTable, Table: "dead", Name: "dead"},
	}
	want := []string{
		`ALTER TABLE "old" RENAME TO "new"`,
		`ALTER TABLE "new" RENAME COLUMN "a" TO "b"`,
		`ALTER TABLE "new" ADD COLUMN "n" INTEGER DEFAULT 7`,
		`CREATE UNIQUE INDEX "ix" ON "new" ("b" DESC, (lower(n))) WHERE n > 1`,
		`DROP INDEX IF EXISTS "gone"`,
		`CREATE VIEW "v" AS SELECT b FROM new`,
		`DROP VIEW IF EXISTS "w"`,
		`CREATE TRIGGER tr AFTER INSERT ON new BEGIN SELECT 1; END`,
		`DROP TRIGGER IF EXISTS "tq"`,
		`DROP TABLE "dead"`,
	}
	stmts, err := Dialect{}.Render(plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(stmts) != len(want) {
		t.Fatalf("got %d statements, want %d: %+v", len(stmts), len(want), stmts)
	}
	for i := range want {
		if stmts[i].SQL != want[i] {
			t.Errorf("statement %d:\n got %s\nwant %s", i, stmts[i].SQL, want[i])
		}
	}
}

func TestSQLiteCreateTableText(t *testing.T) {
	tbl := schema.Table{
		Name: "docs",
		Columns: []schema.Column{
			{Name: "id", Type: "INTEGER", Identity: "by_default"},
			{Name: "title", Type: "TEXT", Default: "''", Collate: "NOCASE"},
			{Name: "status", Type: "TEXT", Nullable: true, Enum: "docs_status"},
			{Name: "tenant", Type: "INTEGER", Nullable: true},
			{Name: "total", Type: "INTEGER", Nullable: true, Generated: "tenant * 2"},
			{Name: "stamp", Type: "TEXT", Default: "datetime('now')"},
		},
		PrimaryKey:  []string{"id"},
		Uniques:     []schema.Unique{{Name: "docs_title_key", Columns: []string{"title"}}},
		Checks:      []schema.Check{{Name: "docs_title_len", Expr: "length(title) > 0"}},
		ForeignKeys: []schema.ForeignKey{{Name: "docs_tenant_fkey", Columns: []string{"tenant"}, RefTable: "tenants", RefColumns: []string{"id"}, OnDelete: "CASCADE"}},
	}
	got, err := createTableSQL("docs", tbl, map[string][]string{"docs_status": {"a", "it's"}})
	if err != nil {
		t.Fatal(err)
	}
	want := `CREATE TABLE "docs" ("id" INTEGER NOT NULL, "title" TEXT COLLATE "NOCASE" DEFAULT '' NOT NULL, "status" TEXT, "tenant" INTEGER, ` +
		`"total" INTEGER GENERATED ALWAYS AS (tenant * 2) STORED, "stamp" TEXT DEFAULT (datetime('now')) NOT NULL, ` +
		`PRIMARY KEY ("id"), CONSTRAINT "docs_title_key" UNIQUE ("title"), CHECK ("status" IN ('a', 'it''s')), ` +
		`CONSTRAINT "docs_title_len" CHECK (length(title) > 0), ` +
		`CONSTRAINT "docs_tenant_fkey" FOREIGN KEY ("tenant") REFERENCES "tenants" ("id") ON DELETE CASCADE)`
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestSQLiteCreateTableRefusals(t *testing.T) {
	cases := map[string]schema.Table{
		"identity off the rowid":  {Name: "t", Columns: []schema.Column{{Name: "id", Type: "TEXT", Identity: "by_default"}}, PrimaryKey: []string{"id"}},
		"serial":                  {Name: "t", Columns: []schema.Column{{Name: "id", Type: "INTEGER", Identity: "serial"}}, PrimaryKey: []string{"id"}},
		"comment":                 {Name: "t", Columns: []schema.Column{{Name: "c", Type: "TEXT", Comment: "x"}}},
		"enum without its values": {Name: "t", Columns: []schema.Column{{Name: "c", Type: "TEXT", Enum: "e"}}},
		"enum on a non-TEXT type": {Name: "t", Columns: []schema.Column{{Name: "c", Type: "INTEGER", Enum: "e"}}},
		"partitioned":             {Name: "t", Columns: []schema.Column{{Name: "c", Type: "TEXT"}}, Partition: &schema.Partition{Strategy: "range", Key: "RANGE (c)"}},
	}
	for name, tbl := range cases {
		if _, err := createTableSQL("t", tbl, nil); err == nil {
			t.Errorf("%s: want an error, got none", name)
		}
	}
}

func TestSQLiteDefaultSQL(t *testing.T) {
	cases := []struct {
		in   string
		want string
		bare bool
	}{
		{"0", "0", true},
		{"-1.5", "-1.5", true},
		{"''", "''", true},
		{"'a''b'", "'a''b'", true},
		{"NULL", "NULL", true},
		{"CURRENT_TIMESTAMP", "CURRENT_TIMESTAMP", true},
		{"x'AB'", "x'AB'", true},
		{"datetime('now')", "(datetime('now'))", false},
		{"1 + 2", "(1 + 2)", false},
		{"(1 + 2)", "(1 + 2)", false},
		{"abc", "(abc)", false},
	}
	for _, c := range cases {
		got, bare := defaultSQL(c.in)
		if got != c.want || bare != c.bare {
			t.Errorf("defaultSQL(%q) = %q, %v; want %q, %v", c.in, got, bare, c.want, c.bare)
		}
	}
}

func TestSQLiteNativeAddRestrictions(t *testing.T) {
	cases := []struct {
		name string
		col  schema.Column
		want bool
	}{
		{"nullable", schema.Column{Name: "c", Type: "TEXT", Nullable: true}, true},
		{"not null with literal default", schema.Column{Name: "c", Type: "TEXT", Default: "'x'"}, true},
		{"not null with NULL default", schema.Column{Name: "c", Type: "TEXT", Default: "NULL"}, false},
		{"current_timestamp default", schema.Column{Name: "c", Type: "TEXT", Nullable: true, Default: "CURRENT_TIMESTAMP"}, false},
		{"expression default", schema.Column{Name: "c", Type: "TEXT", Nullable: true, Default: "lower('A')"}, false},
		{"stored generated", schema.Column{Name: "c", Type: "TEXT", Nullable: true, Generated: "lower('A')"}, false},
		{"enum column", schema.Column{Name: "c", Type: "TEXT", Nullable: true, Enum: "e"}, false},
		{"identity", schema.Column{Name: "c", Type: "INTEGER", Nullable: true, Identity: "by_default"}, false},
	}
	for _, c := range cases {
		if got := nativeAdd(c.col); got != c.want {
			t.Errorf("%s: nativeAdd = %v, want %v", c.name, got, c.want)
		}
	}
}

func liteSchema(tables ...schema.Table) *schema.Schema {
	return &schema.Schema{Dialect: dialect.SQLite, Tables: tables}
}

// TestSQLiteRenderSchemaDropColumn checks that a column nothing uses is dropped
// natively and one an index uses sends the table through a rebuild.
func TestSQLiteRenderSchemaDropColumn(t *testing.T) {
	from := liteSchema(schema.Table{
		Name:       "t",
		Columns:    []schema.Column{{Name: "a", Type: "INTEGER"}, {Name: "b", Type: "TEXT", Nullable: true}, {Name: "c", Type: "TEXT", Nullable: true}},
		PrimaryKey: []string{"a"},
		Indexes:    []schema.Index{{Name: "t_b", Parts: []schema.IndexPart{{Column: "b"}}}},
	})
	to := liteSchema(schema.Table{
		Name:       "t",
		Columns:    []schema.Column{{Name: "a", Type: "INTEGER"}, {Name: "b", Type: "TEXT", Nullable: true}},
		PrimaryKey: []string{"a"},
		Indexes:    []schema.Index{{Name: "t_b", Parts: []schema.IndexPart{{Column: "b"}}}},
	})
	stmts, err := Dialect{}.RenderSchema(from, to, []migrate.Change{{Kind: migrate.DropColumn, Table: "t", Name: "c", Before: &schema.Column{Name: "c"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(stmts) != 1 || stmts[0].SQL != `ALTER TABLE "t" DROP COLUMN "c"` {
		t.Fatalf("an unused column must drop natively, got %+v", stmts)
	}

	to2 := liteSchema(schema.Table{
		Name:       "t",
		Columns:    []schema.Column{{Name: "a", Type: "INTEGER"}, {Name: "c", Type: "TEXT", Nullable: true}},
		PrimaryKey: []string{"a"},
	})
	stmts, err = Dialect{}.RenderSchema(from, to2, []migrate.Change{{Kind: migrate.DropColumn, Table: "t", Name: "b", Before: &schema.Column{Name: "b"}}})
	if err != nil {
		t.Fatal(err)
	}
	var sqls []string
	for _, s := range stmts {
		sqls = append(sqls, s.SQL)
	}
	want := []string{
		`CREATE TABLE "new_t" ("a" INTEGER NOT NULL, "c" TEXT, PRIMARY KEY ("a"))`,
		`INSERT INTO "new_t" ("a", "c") SELECT "a", "c" FROM "t"`,
		`DROP TABLE "t"`,
		`ALTER TABLE "new_t" RENAME TO "t"`,
		`PRAGMA foreign_key_check`,
	}
	if strings.Join(sqls, "\n") != strings.Join(want, "\n") {
		t.Fatalf("an indexed column must rebuild.\ngot:\n%s\nwant:\n%s", strings.Join(sqls, "\n"), strings.Join(want, "\n"))
	}
	if last := stmts[len(stmts)-1]; last.Hint != "fk-check" {
		t.Fatalf("the rebuild must end with the fk-check hint, got %q", last.Hint)
	}
}

// TestSQLiteRenderSchemaRebuildOrder pins the normative statement order of the
// rebuild, including the dependents that are dropped and recreated.
func TestSQLiteRenderSchemaRebuildOrder(t *testing.T) {
	trig := "CREATE TRIGGER items_audit AFTER UPDATE ON items BEGIN SELECT 1; END"
	mk := func(qty schema.Column) *schema.Schema {
		s := liteSchema(schema.Table{
			Name:       "items",
			Columns:    []schema.Column{{Name: "id", Type: "INTEGER"}, {Name: "name", Type: "TEXT", Nullable: true}, qty},
			PrimaryKey: []string{"id"},
			Indexes:    []schema.Index{{Name: "items_name", Parts: []schema.IndexPart{{Column: "name"}}}},
		})
		s.Views = []schema.View{{Name: "items_v", Query: "SELECT id, name FROM items"}}
		s.Triggers = []schema.Trigger{{Name: "items_audit", Table: "items", Definition: trig}}
		return s
	}
	from := mk(schema.Column{Name: "qty", Type: "INTEGER", Nullable: true})
	to := mk(schema.Column{Name: "qty", Type: "INTEGER", Default: "0"})
	plan := []migrate.Change{{Kind: migrate.AlterColumn, Table: "items", Name: "qty", Before: &from.Tables[0].Columns[2], After: &to.Tables[0].Columns[2]}}
	stmts, err := Dialect{}.RenderSchema(from, to, plan)
	if err != nil {
		t.Fatal(err)
	}
	type step struct{ sql, hint string }
	want := []step{
		{`DROP TRIGGER IF EXISTS "items_audit"`, ""},
		{`DROP VIEW IF EXISTS "items_v"`, ""},
		{`CREATE TABLE "new_items" ("id" INTEGER NOT NULL, "name" TEXT, "qty" INTEGER DEFAULT 0 NOT NULL, PRIMARY KEY ("id"))`, ""},
		{`INSERT INTO "new_items" ("id", "name", "qty") SELECT "id", "name", "qty" FROM "items"`, ""},
		{`DROP TABLE "items"`, ""},
		{`ALTER TABLE "new_items" RENAME TO "items"`, ""},
		{`CREATE INDEX "items_name" ON "items" ("name")`, ""},
		{`CREATE VIEW "items_v" AS SELECT id, name FROM items`, "dependent:view:items_v"},
		{`SELECT 1 FROM "items_v" LIMIT 0`, "dependent:view:items_v"},
		{trig, "dependent:trigger:items_audit"},
		{`PRAGMA foreign_key_check`, "fk-check"},
	}
	if len(stmts) != len(want) {
		t.Fatalf("got %d statements, want %d: %+v", len(stmts), len(want), stmts)
	}
	for i, w := range want {
		if stmts[i].SQL != w.sql || stmts[i].Hint != w.hint {
			t.Errorf("statement %d:\n got %q (%s)\nwant %q (%s)", i, stmts[i].SQL, stmts[i].Hint, w.sql, w.hint)
		}
	}
}

// TestSQLiteRebuildFoldsTableChanges checks that every change to a rebuilt table
// is absorbed into one script, at the position of the first rebuild change, and
// that the changes of a dropped table produce nothing of their own.
func TestSQLiteRebuildFoldsTableChanges(t *testing.T) {
	from := liteSchema(
		schema.Table{Name: "a", Columns: []schema.Column{{Name: "id", Type: "INTEGER"}, {Name: "x", Type: "TEXT", Nullable: true}}, PrimaryKey: []string{"id"}},
		schema.Table{Name: "dead", Columns: []schema.Column{{Name: "id", Type: "INTEGER"}, {Name: "a_id", Type: "INTEGER", Nullable: true}}, PrimaryKey: []string{"id"},
			ForeignKeys: []schema.ForeignKey{{Name: "dead_fk", Columns: []string{"a_id"}, RefTable: "a", RefColumns: []string{"id"}}}},
	)
	to := liteSchema(schema.Table{
		Name:       "a",
		Columns:    []schema.Column{{Name: "id", Type: "INTEGER"}, {Name: "y", Type: "TEXT", Nullable: true}, {Name: "z", Type: "TEXT", Nullable: true}},
		PrimaryKey: []string{"id"},
		Indexes:    []schema.Index{{Name: "a_y", Parts: []schema.IndexPart{{Column: "y"}}}},
	})
	plan := []migrate.Change{
		{Kind: migrate.RenameColumn, Table: "a", Name: "y", Before: &schema.Column{Name: "x"}, After: &schema.Column{Name: "y"}},
		{Kind: migrate.DropFK, Table: "dead", Name: "dead_fk"},
		{Kind: migrate.DropTable, Table: "dead", Name: "dead"},
		{Kind: migrate.AddColumn, Table: "a", Name: "z", After: &schema.Column{Name: "z", Type: "TEXT", Nullable: true}},
		{Kind: migrate.AlterColumn, Table: "a", Name: "y", Before: &schema.Column{Name: "y", Type: "TEXT"}, After: &schema.Column{Name: "y", Type: "TEXT", Nullable: true}},
		{Kind: migrate.CreateIndex, Table: "a", Name: "a_y", After: &schema.Index{Name: "a_y", Parts: []schema.IndexPart{{Column: "y"}}}},
	}
	stmts, err := Dialect{}.RenderSchema(from, to, plan)
	if err != nil {
		t.Fatal(err)
	}
	var sqls []string
	for _, s := range stmts {
		sqls = append(sqls, s.SQL)
	}
	want := []string{
		`DROP TABLE "dead"`,
		`CREATE TABLE "new_a" ("id" INTEGER NOT NULL, "y" TEXT, "z" TEXT, PRIMARY KEY ("id"))`,
		`INSERT INTO "new_a" ("id", "y") SELECT "id", "x" FROM "a"`,
		`DROP TABLE "a"`,
		`ALTER TABLE "new_a" RENAME TO "a"`,
		`CREATE INDEX "a_y" ON "a" ("y")`,
		`PRAGMA foreign_key_check`,
	}
	if strings.Join(sqls, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got:\n%s\nwant:\n%s", strings.Join(sqls, "\n"), strings.Join(want, "\n"))
	}
}

// TestSQLiteRenderSchemaEnums checks that enum changes need RenderSchema: it
// writes the enum as a CHECK with the column and rebuilds the tables that use
// an enum that gains a value.
func TestSQLiteRenderSchemaEnums(t *testing.T) {
	mk := func(values ...string) *schema.Schema {
		s := liteSchema(schema.Table{Name: "docs", Columns: []schema.Column{{Name: "id", Type: "INTEGER"}, {Name: "status", Type: "TEXT", Nullable: true, Enum: "docs_status"}}, PrimaryKey: []string{"id"}})
		s.Enums = []schema.Enum{{Name: "docs_status", Values: values}}
		return s
	}
	from, to := mk("a"), mk("a", "b")
	stmts, err := Dialect{}.RenderSchema(from, to, []migrate.Change{{Kind: migrate.AlterEnumAddValue, Name: "docs_status", After: "b"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(stmts) < 2 || stmts[0].SQL != `CREATE TABLE "new_docs" ("id" INTEGER NOT NULL, "status" TEXT, PRIMARY KEY ("id"), CHECK ("status" IN ('a', 'b')))` {
		t.Fatalf("a widened enum must rebuild the table with the new CHECK, got %+v", stmts)
	}

	stmts, err = Dialect{}.RenderSchema(liteSchema(), to, []migrate.Change{
		{Kind: migrate.CreateEnum, Name: "docs_status", After: &to.Enums[0]},
		{Kind: migrate.CreateTable, Table: "docs", Name: "docs", After: &to.Tables[0]},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(stmts) != 1 || !strings.Contains(stmts[0].SQL, `CHECK ("status" IN ('a', 'b'))`) {
		t.Fatalf("create_enum must vanish and the table carry the CHECK, got %+v", stmts)
	}
}

// TestSQLiteRebuildStatementsPure covers RebuildStatements, which does no I/O:
// the common columns are copied, generated ones are not, and an enum column
// without its values is refused.
func TestSQLiteRebuildStatementsPure(t *testing.T) {
	desired := schema.Table{Name: "t", Columns: []schema.Column{
		{Name: "id", Type: "INTEGER"},
		{Name: "added", Type: "TEXT", Nullable: true},
		{Name: "calc", Type: "INTEGER", Nullable: true, Generated: "id + 1"},
	}, PrimaryKey: []string{"id"}}
	stmts, err := RebuildStatements("t", []string{"id", "calc", "gone"}, desired, nil, []Dependent{{Type: "view", Name: "v", Table: "v", SQL: "CREATE VIEW v AS SELECT id FROM t"}})
	if err != nil {
		t.Fatal(err)
	}
	if stmts[len(stmts)-1].SQL != "PRAGMA foreign_key_check" || stmts[len(stmts)-1].Hint != "fk-check" {
		t.Fatalf("the last statement must be the foreign_key_check: %+v", stmts[len(stmts)-1])
	}
	var insert string
	for _, s := range stmts {
		if strings.HasPrefix(s.SQL, "INSERT") {
			insert = s.SQL
		}
	}
	if insert != `INSERT INTO "new_t" ("id") SELECT "id" FROM "t"` {
		t.Fatalf("only the common, non-generated column is copied, got %q", insert)
	}
	if stmts[0].SQL != `DROP VIEW IF EXISTS "v"` {
		t.Fatalf("a dependent is dropped first, got %q", stmts[0].SQL)
	}

	enumCol := schema.Table{Name: "t", Columns: []schema.Column{{Name: "e", Type: "TEXT", Enum: "x"}}}
	if _, err := RebuildStatements("t", []string{"e"}, enumCol, nil, nil); err == nil {
		t.Fatal("an enum column without its values must be refused")
	}
	if _, err := RebuildStatements("t", nil, desired, nil, []Dependent{{Type: "index", Name: "i"}}); err == nil {
		t.Fatal("a dependent that is neither a view nor a trigger must be refused")
	}
}

func TestSQLiteOrderViews(t *testing.T) {
	views := []schema.View{
		{Name: "a_top", Query: "SELECT * FROM b_mid"},
		{Name: "b_mid", Query: "SELECT * FROM c_base"},
		{Name: "c_base", Query: "SELECT * FROM t"},
	}
	var names []string
	for _, v := range orderViews(views) {
		names = append(names, v.Name)
	}
	if strings.Join(names, ",") != "c_base,b_mid,a_top" {
		t.Fatalf("a view must follow the views it reads, got %v", names)
	}
}
