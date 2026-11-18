// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"slices"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/schema"
)

// TestCompanionHistory is issue 252: Config.History declares the history
// table, the trigger function and the trigger as ordinary schema objects.
//
// Catches: a History call that the reader ignores (no history table), objects
// built from something other than the model's final columns, and history added
// to a model that did not ask for it.
func TestCompanionHistory(t *testing.T) {
	g, diags, err := Load([]string{"./companionhistory"}, Config{Dir: fixturesDir(t)})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	s, diags := BuildSchema(g.Models, pg())
	if len(diags) != 0 {
		t.Fatalf("schema diagnostics: %v", diags)
	}
	notes, _ := s.Table("notes")
	want, err := schema.HistoryFor(notes)
	if err != nil {
		t.Fatal(err)
	}
	h, ok := s.Table("notes_history")
	if !ok {
		t.Fatalf("no notes_history table in %+v", s.Tables)
	}
	var names []string
	for _, c := range h.Columns {
		names = append(names, c.Name)
	}
	wantNames := []string{"id", "title", "body", "history_id", "history_operation", "history_changed_at", "history_changed_by"}
	if !slices.Equal(names, wantNames) {
		t.Errorf("notes_history columns = %v, want %v", names, wantNames)
	}
	if !schema.Equal(&schema.Schema{Dialect: dialect.Postgres, Tables: []schema.Table{*h}}, &schema.Schema{Dialect: dialect.Postgres, Tables: []schema.Table{want.Table}}) {
		t.Errorf("notes_history = %+v, want %+v", *h, want.Table)
	}
	if len(s.Functions) != 1 || s.Functions[0].Name != "notes_history_fn" || s.Functions[0].Body != want.Function.Body {
		t.Errorf("functions = %+v, want the notes_history_fn of schema.HistoryFor", s.Functions)
	}
	if len(s.Triggers) != 1 || s.Triggers[0].Name != "notes_history_trg" || s.Triggers[0].Table != "notes" || s.Triggers[0].Function != "notes_history_fn()" {
		t.Errorf("triggers = %+v, want notes_history_trg on notes", s.Triggers)
	}
	if _, ok := s.Table("tags_history"); ok {
		t.Error("tags has no History call but got a history table")
	}
}

// Catches: History reaching a dialect with no transaction-local setting for
// the actor: the migration would be refused late, or would build a trigger
// that cannot know who changed the row.
func TestCompanionHistoryRefusedOutsidePostgres(t *testing.T) {
	g, _, err := Load([]string{"./companionhistory"}, Config{Dir: fixturesDir(t)})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []dialect.Name{dialect.SQLite, dialect.MySQL} {
		s, diags := BuildSchema(g.Models, Config{Dialects: []dialect.Name{d}})
		if joined := diagStrings(diags); !strings.Contains(joined, "model Note: History is not supported on "+string(d)) {
			t.Errorf("%s: no diagnostic for History:\n%s", d, joined)
		}
		if _, ok := s.Table("notes_history"); ok {
			t.Errorf("%s: a history table was declared although History is refused", d)
		}
	}
	_, diags := BuildSchema(g.Models, Config{Dialects: []dialect.Name{dialect.Postgres, dialect.SQLite}})
	if joined := diagStrings(diags); !strings.Contains(joined, "not supported on sqlite") || strings.Contains(joined, "not supported on postgres") {
		t.Errorf("postgres and sqlite: want a diagnostic for sqlite only:\n%s", joined)
	}
}

// Catches: a repeated History call accepted, which would declare the objects
// twice.
func TestCompanionHistoryRepeatIsRefused(t *testing.T) {
	_, diags, err := Load([]string{"./companionhistorybad"}, Config{Dir: fixturesDir(t)})
	if err != nil {
		t.Fatal(err)
	}
	if joined := diagStrings(diags); !strings.Contains(joined, "duplicate History call") {
		t.Errorf("a repeated History call is not refused:\n%s", joined)
	}
}

// Catches: a model that cannot have a history getting a broken one: no key to
// find the versions by, a clash with a history column or with another table, a
// schema the trigger function would not live in, and a name PostgreSQL would
// silently cut.
func TestCompanionHistoryRefusals(t *testing.T) {
	g, diags, err := Load([]string{"./companionhistoryrefused"}, Config{Dir: fixturesDir(t)})
	if err != nil {
		t.Fatal(err)
	}
	joined := diagStrings(diags)
	for _, want := range []string{
		`model NoKey: History: table "no_keys" has no primary key`,
		`model Reserved: History: table "reserveds" has a column named "history_id"`,
		`model Owner: History: the history table "owners_history" is already the table of model OwnersHistory`,
		`model Scoped: History: table "scoped" is in the schema "app"`,
		`model LongName: History: the derived name`,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing diagnostic %q in:\n%s", want, joined)
		}
	}
	s, _ := BuildSchema(g.Models, pg())
	for _, name := range []string{"no_keys_history", "reserveds_history", "scoped_history"} {
		if _, ok := s.Table(name); ok {
			t.Errorf("a refused history declared the table %s", name)
		}
	}
}
