// SPDX-License-Identifier: Apache-2.0

package postgres_test

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/migrate"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/schema"
)

func TestRenderCreateSequence(t *testing.T) {
	cases := []struct {
		name string
		seq  schema.Sequence
		want string
	}{
		{
			"defaults",
			schema.Sequence{Name: "jobs_seq"},
			`CREATE SEQUENCE "jobs_seq" AS bigint INCREMENT BY 1 MINVALUE 1 MAXVALUE 9223372036854775807 START WITH 1 CACHE 1 NO CYCLE`,
		},
		{
			"options",
			schema.Sequence{Schema: "app", Name: "ids", AsType: "integer", Start: 100, Increment: 10, MinValue: 100, MaxValue: 1000, Cache: 5, Cycle: true},
			`CREATE SEQUENCE "app"."ids" AS integer INCREMENT BY 10 MINVALUE 100 MAXVALUE 1000 START WITH 100 CACHE 5 CYCLE`,
		},
		{
			"descending",
			schema.Sequence{Name: "down", AsType: "smallint", Increment: -1},
			`CREATE SEQUENCE "down" AS smallint INCREMENT BY -1 MINVALUE -32768 MAXVALUE -1 START WITH -1 CACHE 1 NO CYCLE`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st := renderSQL(t, migrate.Change{Kind: migrate.CreateSequence, Schema: c.seq.Schema, Name: c.seq.Name, After: &c.seq})
			if len(st) != 1 || st[0].SQL != c.want {
				t.Fatalf("got  %+v\nwant %q", st, c.want)
			}
		})
	}
}

// TestRenderAlterAndDropSequence: an alter spells every option out, so one the
// declaration no longer sets goes back to its default, and a drop names the
// schema.
func TestRenderAlterAndDropSequence(t *testing.T) {
	q := schema.Sequence{Name: "jobs_seq", Increment: 5}
	st := renderSQL(t, migrate.Change{Kind: migrate.AlterSequence, Name: "jobs_seq", Before: &schema.Sequence{Name: "jobs_seq"}, After: &q})
	want := `ALTER SEQUENCE "jobs_seq" AS bigint INCREMENT BY 5 MINVALUE 1 MAXVALUE 9223372036854775807 START WITH 1 CACHE 1 NO CYCLE`
	if len(st) != 1 || st[0].SQL != want {
		t.Fatalf("got  %+v\nwant %q", st, want)
	}
	st = renderSQL(t, migrate.Change{Kind: migrate.DropSequence, Schema: "app", Name: "jobs_seq", Before: &q})
	if len(st) != 1 || st[0].SQL != `DROP SEQUENCE "app"."jobs_seq"` {
		t.Fatalf("got %+v", st)
	}
}

func TestRenderSequenceRefusesUnknownType(t *testing.T) {
	_, err := mpg.Dialect{}.Render([]migrate.Change{{Kind: migrate.CreateSequence, Name: "s", After: &schema.Sequence{Name: "s", AsType: "text"}}})
	if err == nil || !strings.Contains(err.Error(), "smallint, integer or bigint") {
		t.Fatalf("want a type error, got %v", err)
	}
}

// TestRenderSequencePlanOrder renders an ordered plan: the sequence comes
// before the table whose default names it, and a dropped one after.
//
// Catches: an init migration that creates the table first and fails on an
// empty database with `relation "jobs_seq" does not exist`.
func TestRenderSequencePlanOrder(t *testing.T) {
	to := &schema.Schema{Dialect: "postgres", Sequences: []schema.Sequence{{Name: "jobs_seq"}}, Tables: []schema.Table{{
		Name:       "jobs",
		Columns:    []schema.Column{{Name: "id", Type: "bigint", Default: "nextval('jobs_seq'::regclass)"}},
		PrimaryKey: []string{"id"},
	}}}
	changes, _, err := migrate.Diff(&schema.Schema{Dialect: "postgres"}, to, migrate.DiffOptions{})
	if err != nil {
		t.Fatal(err)
	}
	ordered, err := migrate.OrderErr(changes)
	if err != nil {
		t.Fatal(err)
	}
	st := renderSQL(t, ordered...)
	if len(st) != 2 || !strings.HasPrefix(st[0].SQL, `CREATE SEQUENCE "jobs_seq"`) || !strings.HasPrefix(st[1].SQL, `CREATE TABLE "jobs"`) {
		t.Fatalf("got %+v", st)
	}
}
