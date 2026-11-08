// SPDX-License-Identifier: Apache-2.0

package mysql_test

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/migrate"
	mmysql "github.com/azrtydxb/momo/migrate/mysql"
	"github.com/azrtydxb/momo/schema"
)

func render(t *testing.T, c migrate.Change) []string {
	t.Helper()
	stmts, err := mmysql.New().Render([]migrate.Change{c})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	out := make([]string, len(stmts))
	for i, s := range stmts {
		if s.NoTx {
			t.Errorf("statement %q is NoTx; every MySQL migration is non-transactional already", s.SQL)
		}
		out[i] = s.SQL
	}
	return out
}

func TestRenderCreateTable(t *testing.T) {
	tbl := schema.Table{
		Name:       "children",
		PrimaryKey: []string{"id"},
		Columns: []schema.Column{
			{Name: "id", Type: "bigint", Identity: "by_default"},
			{Name: "pa", Type: "int"},
			{Name: "qty", Type: "int", Default: "0"},
			{Name: "total", Type: "int", Nullable: true, Generated: "qty * 2"},
			{Name: "note", Type: "varchar(40)", Nullable: true, Default: "'it''s'", Comment: "a note"},
			{Name: "created", Type: "datetime(6)", Default: "CURRENT_TIMESTAMP(6)"},
			{Name: "token", Type: "varchar(36)", Default: "uuid()"},
		},
		Uniques:     []schema.Unique{{Name: "children_pa", Columns: []string{"pa"}}},
		Checks:      []schema.Check{{Name: "qty_nonnegative", Expr: "qty >= 0"}},
		ForeignKeys: []schema.ForeignKey{{Name: "children_parent", Columns: []string{"pa"}, RefTable: "parents", RefColumns: []string{"a"}, OnDelete: "cascade"}},
	}
	got := render(t, migrate.Change{Kind: migrate.CreateTable, Table: "children", After: tbl})
	want := "CREATE TABLE `children` (" +
		"`id` bigint NOT NULL AUTO_INCREMENT, " +
		"`pa` int NOT NULL, " +
		"`qty` int NOT NULL DEFAULT 0, " +
		"`total` int GENERATED ALWAYS AS (qty * 2) STORED, " +
		"`note` varchar(40) NULL DEFAULT 'it''s' COMMENT 'a note', " +
		"`created` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6), " +
		"`token` varchar(36) NOT NULL DEFAULT (uuid()), " +
		"PRIMARY KEY (`id`), " +
		"CONSTRAINT `children_pa` UNIQUE (`pa`), " +
		"CONSTRAINT `qty_nonnegative` CHECK (qty >= 0), " +
		"CONSTRAINT `children_parent` FOREIGN KEY (`pa`) REFERENCES `parents` (`a`) ON DELETE CASCADE" +
		") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
	if len(got) != 1 || got[0] != want {
		t.Fatalf("CREATE TABLE =\n%q\nwant\n%q", got, want)
	}
}

func TestRenderAlterStatements(t *testing.T) {
	ix := schema.Index{Name: "docs_slug", Unique: true, Parts: []schema.IndexPart{{Column: "slug(20)"}, {Column: "created", Desc: true}, {Expr: "lower(title)"}}}
	cases := []struct {
		name string
		c    migrate.Change
		want []string
	}{
		{"add column", migrate.Change{Kind: migrate.AddColumn, Table: "t", After: schema.Column{Name: "c", Type: "int", Nullable: true}},
			[]string{"ALTER TABLE `t` ADD COLUMN `c` int NULL"}},
		{"drop column", migrate.Change{Kind: migrate.DropColumn, Table: "t", Name: "c"},
			[]string{"ALTER TABLE `t` DROP COLUMN `c`"}},
		{"rename column", migrate.Change{Kind: migrate.RenameColumn, Table: "t", Name: "d", Before: schema.Column{Name: "c"}},
			[]string{"ALTER TABLE `t` RENAME COLUMN `c` TO `d`"}},
		{"rename table", migrate.Change{Kind: migrate.RenameTable, Table: "t", Name: "u"},
			[]string{"RENAME TABLE `t` TO `u`"}},
		{"type change modifies the column", migrate.Change{Kind: migrate.AlterColumn, Table: "t",
			Before: schema.Column{Name: "c", Type: "int"}, After: schema.Column{Name: "c", Type: "bigint"}},
			[]string{"ALTER TABLE `t` MODIFY COLUMN `c` bigint NOT NULL"}},
		{"integer display width is no change of type", migrate.Change{Kind: migrate.AlterColumn, Table: "t",
			Before: schema.Column{Name: "c", Type: "int(11)", Default: "1"}, After: schema.Column{Name: "c", Type: "int", Default: "2"}},
			[]string{"ALTER TABLE `t` ALTER COLUMN `c` SET DEFAULT 2"}},
		{"drop default", migrate.Change{Kind: migrate.AlterColumn, Table: "t",
			Before: schema.Column{Name: "c", Type: "int", Default: "1"}, After: schema.Column{Name: "c", Type: "int"}},
			[]string{"ALTER TABLE `t` ALTER COLUMN `c` DROP DEFAULT"}},
		{"add primary key", migrate.Change{Kind: migrate.AddPrimaryKey, Table: "t", After: []string{"a", "b"}},
			[]string{"ALTER TABLE `t` ADD PRIMARY KEY (`a`, `b`)"}},
		{"drop primary key", migrate.Change{Kind: migrate.DropPrimaryKey, Table: "t", Name: "PRIMARY"},
			[]string{"ALTER TABLE `t` DROP PRIMARY KEY"}},
		{"add unique", migrate.Change{Kind: migrate.AddUnique, Table: "t", After: schema.Unique{Name: "u", Columns: []string{"a"}}},
			[]string{"ALTER TABLE `t` ADD CONSTRAINT `u` UNIQUE (`a`)"}},
		{"drop unique", migrate.Change{Kind: migrate.DropUnique, Table: "t", Name: "u"},
			[]string{"ALTER TABLE `t` DROP INDEX `u`"}},
		{"add check", migrate.Change{Kind: migrate.AddCheck, Table: "t", After: schema.Check{Name: "k", Expr: "a > 0"}},
			[]string{"ALTER TABLE `t` ADD CONSTRAINT `k` CHECK (a > 0)"}},
		{"drop check", migrate.Change{Kind: migrate.DropCheck, Table: "t", Name: "k"},
			[]string{"ALTER TABLE `t` DROP CONSTRAINT `k`"}},
		{"add foreign key", migrate.Change{Kind: migrate.AddFK, Table: "t", After: schema.ForeignKey{Name: "f", Columns: []string{"a"}, RefTable: "p", RefColumns: []string{"id"}, OnUpdate: "SET NULL"}},
			[]string{"ALTER TABLE `t` ADD CONSTRAINT `f` FOREIGN KEY (`a`) REFERENCES `p` (`id`) ON UPDATE SET NULL"}},
		{"drop foreign key", migrate.Change{Kind: migrate.DropFK, Table: "t", Name: "f"},
			[]string{"ALTER TABLE `t` DROP FOREIGN KEY `f`"}},
		{"create index", migrate.Change{Kind: migrate.CreateIndex, Table: "docs", After: ix},
			[]string{"CREATE UNIQUE INDEX `docs_slug` ON `docs` (`slug`(20), `created` DESC, (lower(title)))"}},
		{"fulltext index", migrate.Change{Kind: migrate.CreateIndex, Table: "docs", After: schema.Index{Name: "docs_ft", Method: "fulltext", Parts: []schema.IndexPart{{Column: "body"}}}},
			[]string{"CREATE FULLTEXT INDEX `docs_ft` ON `docs` (`body`)"}},
		{"drop index", migrate.Change{Kind: migrate.DropIndex, Table: "docs", Name: "docs_slug"},
			[]string{"DROP INDEX `docs_slug` ON `docs`"}},
		{"drop table", migrate.Change{Kind: migrate.DropTable, Table: "docs"},
			[]string{"DROP TABLE `docs`"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := render(t, c.c)
			if strings.Join(got, ";") != strings.Join(c.want, ";") {
				t.Fatalf("got  %q\nwant %q", got, c.want)
			}
		})
	}
}

func TestRenderRefusesWhatMySQLLacks(t *testing.T) {
	cases := map[string]migrate.Change{
		"partial index": {Kind: migrate.CreateIndex, Table: "t", After: schema.Index{Name: "i", Where: "a > 0", Parts: []schema.IndexPart{{Column: "a"}}}},
		"gin index":     {Kind: migrate.CreateIndex, Table: "t", After: schema.Index{Name: "i", Method: "gin", Parts: []schema.IndexPart{{Column: "a"}}}},
		"always identity": {Kind: migrate.AddColumn, Table: "t",
			After: schema.Column{Name: "c", Type: "bigint", Identity: "always"}},
		"serial":  {Kind: migrate.AddColumn, Table: "t", After: schema.Column{Name: "c", Type: "integer", Identity: "serial"}},
		"enum":    {Kind: migrate.CreateEnum, Name: "mood", After: schema.Enum{Name: "mood", Values: []string{"ok"}}},
		"view":    {Kind: migrate.CreateView, Name: "v", After: schema.View{Name: "v", Query: "select 1"}},
		"unknown": {Kind: "no_such_kind"},
		"collate injection": {Kind: migrate.AddColumn, Table: "t",
			After: schema.Column{Name: "c", Type: "varchar(5)", Collate: "x; DROP TABLE t"}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := mmysql.New().Render([]migrate.Change{c}); err == nil {
				t.Fatal("Render succeeded, want an error")
			}
		})
	}
}

func TestHistoryDDLQualifiesTheTable(t *testing.T) {
	ddl := mmysql.New().HistoryDDLFor("`app`.`momo_migrations`")
	if len(ddl) != 1 || !strings.HasPrefix(ddl[0], "CREATE TABLE IF NOT EXISTS `app`.`momo_migrations` (") {
		t.Fatalf("HistoryDDLFor = %q", ddl)
	}
	if !strings.Contains(ddl[0], "version VARCHAR(255) NOT NULL PRIMARY KEY") {
		t.Fatalf("the version key must be a VARCHAR, MySQL cannot index TEXT: %q", ddl[0])
	}
}

// The engine finds the history table and the lock key through these.
var (
	_ migrate.Dialect        = mmysql.New()
	_ migrate.HistoryLocator = mmysql.New()
)
