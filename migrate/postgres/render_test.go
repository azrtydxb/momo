// SPDX-License-Identifier: Apache-2.0

package postgres_test

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/migrate"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/schema"
)

func renderSQL(t *testing.T, c ...migrate.Change) []migrate.Statement {
	t.Helper()
	out, err := mpg.Dialect{}.Render(c)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	return out
}

func TestRenderEnumAddValueHint(t *testing.T) {
	st := renderSQL(t, migrate.Change{Kind: migrate.AlterEnumAddValue, Name: "status", Before: &schema.Enum{Name: "status", Values: []string{"a"}}, After: "it's"})
	if len(st) != 1 || st[0].SQL != `ALTER TYPE "status" ADD VALUE 'it''s'` || st[0].Hint != "enum-add" {
		t.Fatalf("got %+v", st)
	}
}

func TestRenderDropEnumUnsupported(t *testing.T) {
	_, err := mpg.Dialect{}.Render([]migrate.Change{{Kind: migrate.DropEnum, Name: "status", Before: &schema.Enum{Name: "status"}}})
	if err == nil || !strings.Contains(err.Error(), "status") {
		t.Fatalf("want an error naming the enum, got %v", err)
	}
}

func TestRenderQualifiesAndQuotes(t *testing.T) {
	st := renderSQL(t, migrate.Change{Kind: migrate.DropColumn, Schema: "app", Table: `we"ird`, Name: "c"})
	want := `ALTER TABLE "app"."we""ird" DROP COLUMN "c"`
	if st[0].SQL != want {
		t.Fatalf("got %q want %q", st[0].SQL, want)
	}
	st = renderSQL(t, migrate.Change{Kind: migrate.DropColumn, Table: "t", Name: "c"})
	if st[0].SQL != `ALTER TABLE "t" DROP COLUMN "c"` {
		t.Fatalf("got %q", st[0].SQL)
	}
}

func TestRenderCommentAndCollate(t *testing.T) {
	col := &schema.Column{Name: "name", Type: "text", Collate: "C", Comment: `it's a \ path`}
	st := renderSQL(t, migrate.Change{Kind: migrate.AddColumn, Table: "t", Name: "name", After: col})
	if len(st) != 2 {
		t.Fatalf("want add + comment, got %+v", st)
	}
	if st[0].SQL != `ALTER TABLE "t" ADD COLUMN "name" text COLLATE "C" NOT NULL` {
		t.Fatalf("add: %q", st[0].SQL)
	}
	if st[1].SQL != `COMMENT ON COLUMN "t"."name" IS E'it''s a \\ path'` {
		t.Fatalf("comment: %q", st[1].SQL)
	}
	before := &schema.Column{Name: "name", Type: "text", Comment: "old"}
	after := &schema.Column{Name: "name", Type: "text"}
	st = renderSQL(t, migrate.Change{Kind: migrate.AlterColumn, Table: "t", Name: "name", Before: before, After: after})
	if len(st) != 1 || st[0].SQL != `COMMENT ON COLUMN "t"."name" IS NULL` {
		t.Fatalf("clearing a comment: %+v", st)
	}
	st = renderSQL(t, migrate.Change{Kind: migrate.AlterColumn, Table: "t", Name: "name",
		Before: &schema.Column{Name: "name", Type: "text"}, After: &schema.Column{Name: "name", Type: "text", Collate: "C"}})
	if len(st) != 1 || st[0].SQL != `ALTER TABLE "t" ALTER COLUMN "name" TYPE text COLLATE "C" USING "name"::text` {
		t.Fatalf("collate change: %+v", st)
	}
}

func TestRenderForeignKeyActions(t *testing.T) {
	fk := &schema.ForeignKey{Name: "fk", Columns: []string{"o"}, RefTable: "orgs", RefColumns: []string{"id"}, OnDelete: "CASCADE", OnUpdate: "SET NULL"}
	st := renderSQL(t, migrate.Change{Kind: migrate.AddFK, Table: "docs", Name: "fk", After: fk})
	want := `ALTER TABLE "docs" ADD CONSTRAINT "fk" FOREIGN KEY ("o") REFERENCES "orgs" ("id") ON DELETE CASCADE ON UPDATE SET NULL`
	if st[0].SQL != want {
		t.Fatalf("got %q want %q", st[0].SQL, want)
	}
	bad := *fk
	bad.OnDelete = "CASCADE; DROP TABLE x"
	if _, err := (mpg.Dialect{}).Render([]migrate.Change{{Kind: migrate.AddFK, Table: "docs", Name: "fk", After: &bad}}); err == nil {
		t.Fatal("an unknown FK action must be refused")
	}
}

func TestRenderFunctions(t *testing.T) {
	f := &schema.Function{Name: "touch", Returns: "trigger", Language: "plpgsql", Volatility: "VOLATILE", Body: "BEGIN RETURN NEW; END;"}
	st := renderSQL(t, migrate.Change{Kind: migrate.CreateFunction, Name: "touch", After: f})
	if !strings.HasPrefix(st[0].SQL, `CREATE FUNCTION "touch"() RETURNS trigger LANGUAGE "plpgsql" VOLATILE AS $momo$`) || !strings.HasSuffix(st[0].SQL, "$momo$") {
		t.Fatalf("create: %q", st[0].SQL)
	}
	g := *f
	g.Body = "BEGIN RETURN OLD; END;"
	st = renderSQL(t, migrate.Change{Kind: migrate.ReplaceFunction, Name: "touch", Before: f, After: &g})
	if len(st) != 1 || !strings.HasPrefix(st[0].SQL, `CREATE OR REPLACE FUNCTION "touch"()`) {
		t.Fatalf("replace: %+v", st)
	}
	h := g
	h.Returns = "void"
	st = renderSQL(t, migrate.Change{Kind: migrate.ReplaceFunction, Name: "touch", Before: f, After: &h})
	if len(st) != 2 || st[0].SQL != `DROP FUNCTION "touch"()` || !strings.HasPrefix(st[1].SQL, `CREATE FUNCTION "touch"()`) {
		t.Fatalf("changed return type must drop then create: %+v", st)
	}
	evil := *f
	evil.Body = "SELECT '$momo$'"
	if _, err := (mpg.Dialect{}).Render([]migrate.Change{{Kind: migrate.CreateFunction, Name: "touch", After: &evil}}); err == nil {
		t.Fatal("a body containing the dollar tag must be refused")
	}
}

func TestRenderTriggerPolicyRLS(t *testing.T) {
	tr := &schema.Trigger{Table: "events", Name: "t1", Timing: "BEFORE", Events: []string{"INSERT", "UPDATE"}, ForEachRow: true, When: "NEW.x > 0", Function: "touch()", Args: []string{"a'b"}}
	st := renderSQL(t, migrate.Change{Kind: migrate.CreateTrigger, Table: "events", Name: "t1", After: tr})
	want := `CREATE TRIGGER "t1" BEFORE INSERT OR UPDATE ON "events" FOR EACH ROW WHEN (NEW.x > 0) EXECUTE FUNCTION touch('a''b')`
	if st[0].SQL != want {
		t.Fatalf("got %q want %q", st[0].SQL, want)
	}
	p := &schema.Policy{Table: "docs", Name: "p", Command: "SELECT", Permissive: true, Roles: []string{"app"}, Using: "true", Check: "true"}
	st = renderSQL(t, migrate.Change{Kind: migrate.CreatePolicy, Table: "docs", Name: "p", After: p})
	wantP := `CREATE POLICY "p" ON "docs" AS PERMISSIVE FOR SELECT TO "app" USING (true) WITH CHECK (true)`
	if st[0].SQL != wantP {
		t.Fatalf("got %q want %q", st[0].SQL, wantP)
	}
	st = renderSQL(t, migrate.Change{Kind: migrate.SetRLS, Table: "docs", Name: "docs", After: &schema.RLS{Enabled: true, Forced: true}})
	if len(st) != 2 || st[0].SQL != `ALTER TABLE "docs" ENABLE ROW LEVEL SECURITY` || st[1].SQL != `ALTER TABLE "docs" FORCE ROW LEVEL SECURITY` {
		t.Fatalf("rls: %+v", st)
	}
}

func TestRenderPartitions(t *testing.T) {
	cols := []schema.Column{{Name: "id", Type: "bigint"}}
	parent := &schema.Table{Name: "events", Columns: cols, Partition: &schema.Partition{Strategy: "range", Key: "RANGE (id)"}}
	st := renderSQL(t, migrate.Change{Kind: migrate.CreateTable, Table: "events", Name: "events", After: parent})
	if st[0].SQL != `CREATE TABLE "events" ("id" bigint NOT NULL) PARTITION BY RANGE (id)` {
		t.Fatalf("parent: %q", st[0].SQL)
	}
	child := &schema.Table{Name: "events_1", Columns: cols, PartitionOf: &schema.PartitionOf{Parent: "events", Bound: "FOR VALUES FROM (1) TO (2)"}}
	st = renderSQL(t, migrate.Change{Kind: migrate.CreateTable, Table: "events_1", Name: "events_1", After: child})
	if st[0].SQL != `CREATE TABLE "events_1" PARTITION OF "events" FOR VALUES FROM (1) TO (2)` {
		t.Fatalf("child: %q", st[0].SQL)
	}
}

func TestRenderViews(t *testing.T) {
	v := &schema.View{Name: "v", Query: "SELECT 1;\n", Materialized: true, Indexes: []schema.Index{{Name: "v_i", Parts: []schema.IndexPart{{Column: "x"}}}}}
	st := renderSQL(t, migrate.Change{Kind: migrate.CreateView, Name: "v", After: v})
	if len(st) != 2 || st[0].SQL != `CREATE MATERIALIZED VIEW "v" AS SELECT 1` || st[1].SQL != `CREATE INDEX "v_i" ON "v" ("x")` {
		t.Fatalf("matview: %+v", st)
	}
	st = renderSQL(t, migrate.Change{Kind: migrate.DropView, Name: "v", Before: v})
	if st[0].SQL != `DROP MATERIALIZED VIEW "v"` {
		t.Fatalf("drop: %q", st[0].SQL)
	}
}

func TestRenderIndexVariants(t *testing.T) {
	ix := &schema.Index{Name: "e", Unique: true, Method: "hnsw", Where: "x > 0",
		Parts: []schema.IndexPart{{Column: "emb", OpClass: "vector_cosine_ops"}, {Expr: "lower(y)", Desc: true, Nulls: "last"}},
		With:  []schema.KV{{Key: "m", Value: "16"}}}
	st := renderSQL(t, migrate.Change{Kind: migrate.CreateIndex, Table: "d", Name: "e", After: ix})
	want := `CREATE UNIQUE INDEX "e" ON "d" USING "hnsw" ("emb" "vector_cosine_ops", (lower(y)) DESC NULLS LAST) WITH ("m" = 16) WHERE x > 0`
	if st[0].SQL != want || st[0].NoTx {
		t.Fatalf("got %+v want %q", st[0], want)
	}
	st = renderSQL(t, migrate.Change{Kind: migrate.DropIndex, Schema: "app", Table: "d", Name: "e", Before: &schema.Index{Name: "e", Concurrently: true}})
	if st[0].SQL != `DROP INDEX CONCURRENTLY "app"."e"` || !st[0].NoTx {
		t.Fatalf("drop concurrently: %+v", st[0])
	}
}

func TestRenderDeterministic(t *testing.T) {
	tbl := &schema.Table{Name: "t", Columns: []schema.Column{{Name: "a", Type: "int", Default: "1"}, {Name: "b", Type: "text", Nullable: true}},
		PrimaryKey: []string{"a"}, Uniques: []schema.Unique{{Name: "u", Columns: []string{"b"}}}, Checks: []schema.Check{{Name: "c", Expr: "a > 0"}}}
	c := migrate.Change{Kind: migrate.CreateTable, Table: "t", Name: "t", After: tbl}
	a := renderSQL(t, c)
	for range 5 {
		b := renderSQL(t, c)
		if len(a) != len(b) || a[0].SQL != b[0].SQL {
			t.Fatalf("not deterministic:\n%q\n%q", a[0].SQL, b[0].SQL)
		}
	}
	want := `CREATE TABLE "t" ("a" int DEFAULT 1 NOT NULL, "b" text, CONSTRAINT "t_pkey" PRIMARY KEY ("a"), CONSTRAINT "u" UNIQUE ("b"), CONSTRAINT "c" CHECK (a > 0))`
	if a[0].SQL != want {
		t.Fatalf("got %q want %q", a[0].SQL, want)
	}
}

func TestRenderUnknownKindErrors(t *testing.T) {
	if _, err := (mpg.Dialect{}).Render([]migrate.Change{{Kind: "no_such_kind", Name: "x"}}); err == nil {
		t.Fatal("an unknown change kind must be an error")
	}
}

func TestRenderFunctionBodyTagCollision(t *testing.T) {
	for _, body := range []string{"SELECT '$momo$'", "SELECT 1 -- $momo", "$momo"} {
		f := &schema.Function{Name: "f", Returns: "int", Language: "sql", Body: body}
		if _, err := (mpg.Dialect{}).Render([]migrate.Change{{Kind: migrate.CreateFunction, Name: "f", After: f}}); err == nil {
			t.Fatalf("body %q would collide with the dollar tag and must be refused", body)
		}
	}
}

func TestRenderPolicyRoleKeywordsCaseInsensitive(t *testing.T) {
	p := &schema.Policy{Table: "d", Name: "p", Command: "ALL", Permissive: true, Roles: []string{"public", "Current_User", "app"}}
	st := renderSQL(t, migrate.Change{Kind: migrate.CreatePolicy, Table: "d", Name: "p", After: p})
	want := `CREATE POLICY "p" ON "d" AS PERMISSIVE FOR ALL TO PUBLIC, CURRENT_USER, "app"`
	if st[0].SQL != want {
		t.Fatalf("got %q want %q", st[0].SQL, want)
	}
}

func TestRenderTriggerFunctionValidated(t *testing.T) {
	ok := func(fn string) error {
		tr := &schema.Trigger{Table: "d", Name: "t", Timing: "AFTER", Events: []string{"INSERT"}, Function: fn}
		_, err := (mpg.Dialect{}).Render([]migrate.Change{{Kind: migrate.CreateTrigger, Table: "d", Name: "t", After: tr}})
		return err
	}
	for _, fn := range []string{"touch()", `app.touch()`, `"My Fn"()`, `"app"."my fn"()`} {
		if err := ok(fn); err != nil {
			t.Fatalf("%q must be accepted: %v", fn, err)
		}
	}
	for _, fn := range []string{"touch", "touch(); DROP TABLE x", "a.b.c()", "touch(text)"} {
		if err := ok(fn); err == nil {
			t.Fatalf("%q must be refused", fn)
		}
	}
}

func TestRenderFunctionStrictSecurityDefiner(t *testing.T) {
	base := schema.Function{Name: "twice", Args: "x integer", Returns: "integer", Language: "sql", Volatility: "IMMUTABLE", Body: "SELECT x * 2"}
	plain := renderSQL(t, migrate.Change{Kind: migrate.CreateFunction, Name: "twice", After: &base})[0].SQL
	if strings.Contains(plain, "STRICT") || strings.Contains(plain, "SECURITY") {
		t.Fatalf("flags unset must emit neither clause: %q", plain)
	}
	both := base
	both.Strict, both.SecurityDefiner = true, true
	got := renderSQL(t, migrate.Change{Kind: migrate.CreateFunction, Name: "twice", After: &both})[0].SQL
	if !strings.Contains(got, `IMMUTABLE STRICT SECURITY DEFINER AS $momo$`) {
		t.Fatalf("create with both flags: %q", got)
	}
	strict := base
	strict.Strict = true
	got = renderSQL(t, migrate.Change{Kind: migrate.CreateFunction, Name: "twice", After: &strict})[0].SQL
	if !strings.Contains(got, "STRICT AS") || strings.Contains(got, "SECURITY") {
		t.Fatalf("strict only: %q", got)
	}
	def := base
	def.SecurityDefiner = true
	st := renderSQL(t, migrate.Change{Kind: migrate.ReplaceFunction, Name: "twice", Before: &base, After: &def})
	if len(st) != 1 || !strings.HasPrefix(st[0].SQL, "CREATE OR REPLACE FUNCTION") || !strings.Contains(st[0].SQL, "SECURITY DEFINER AS") || strings.Contains(st[0].SQL, "STRICT") {
		t.Fatalf("replace with security definer: %+v", st)
	}
}
