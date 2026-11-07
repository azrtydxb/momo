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
	for _, pair := range [][2]string{{"by_default", "serial"}, {"serial", "by_default"}, {"", "serial"}, {"serial", ""}} {
		before := &schema.Column{Name: "id", Type: "bigint", Identity: pair[0]}
		after := &schema.Column{Name: "id", Type: "bigint", Identity: pair[1]}
		_, err := mpg.Dialect{}.Render([]migrate.Change{{Kind: migrate.AlterColumn, Table: "t", Name: "id", Before: before, After: after}})
		if err == nil || !strings.Contains(err.Error(), "hand-written migration") {
			t.Fatalf("%q to %q: want a refusal, got %v", pair[0], pair[1], err)
		}
	}
}
