// SPDX-License-Identifier: Apache-2.0

package postgres_test

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/migrate"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/schema"
)

func TestRenderSerialCreateTable(t *testing.T) {
	tbl := &schema.Table{
		Name: "events",
		Columns: []schema.Column{
			{Name: "id", Type: "bigint", Identity: "serial"},
			{Name: "seq", Type: "integer", Identity: "serial"},
			{Name: "small", Type: "smallint", Identity: "serial"},
			{Name: "name", Type: "text"},
		},
		PrimaryKey: []string{"id"},
	}
	st := renderSQL(t, migrate.Change{Kind: migrate.CreateTable, Table: "events", Name: "events", After: tbl})
	want := `CREATE TABLE "events" ("id" bigserial NOT NULL, "seq" serial NOT NULL, "small" smallserial NOT NULL, "name" text NOT NULL, CONSTRAINT "events_pkey" PRIMARY KEY ("id"))`
	if st[0].SQL != want {
		t.Fatalf("got  %q\nwant %q", st[0].SQL, want)
	}
}

func TestRenderSerialAddColumn(t *testing.T) {
	col := &schema.Column{Name: "n", Type: "integer", Identity: "serial"}
	st := renderSQL(t, migrate.Change{Kind: migrate.AddColumn, Table: "t", Name: "n", After: col})
	if st[0].SQL != `ALTER TABLE "t" ADD COLUMN "n" serial NOT NULL` {
		t.Fatalf("got %q", st[0].SQL)
	}
}

func TestRenderSerialNeedsIntegerType(t *testing.T) {
	col := &schema.Column{Name: "n", Type: "text", Identity: "serial"}
	_, err := mpg.Dialect{}.Render([]migrate.Change{{Kind: migrate.AddColumn, Table: "t", Name: "n", After: col}})
	if err == nil || !strings.Contains(err.Error(), "serial needs") {
		t.Fatalf("want a serial type error, got %v", err)
	}
}

func TestRenderSerialConversionRefused(t *testing.T) {
	cases := []struct {
		before, after schema.Column
		want          string
	}{
		{
			schema.Column{Name: "id", Type: "bigint", Identity: "serial"},
			schema.Column{Name: "id", Type: "bigint", Identity: "by_default"},
			`column "id" is a serial column in the database but the model declares identity: change the tag to serial (or write a migration converting it)`,
		},
		{
			schema.Column{Name: "id", Type: "bigint", Identity: "by_default"},
			schema.Column{Name: "id", Type: "bigint", Identity: "serial"},
			`column "id" is an identity column in the database but the model declares serial: change the tag to identity (or write a migration converting it)`,
		},
		{
			schema.Column{Name: "id", Type: "bigint"},
			schema.Column{Name: "id", Type: "bigint", Identity: "serial"},
			"needs a hand-written migration",
		},
		{
			schema.Column{Name: "id", Type: "bigint", Identity: "serial"},
			schema.Column{Name: "id", Type: "integer", Identity: "by_default"},
			"needs a hand-written migration",
		},
	}
	for _, tc := range cases {
		before, after := tc.before, tc.after
		_, err := mpg.Dialect{}.Render([]migrate.Change{{Kind: migrate.AlterColumn, Table: "t", Name: "id", Before: &before, After: &after}})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%+v to %+v: want %q, got %v", tc.before, tc.after, tc.want, err)
		}
	}
}
