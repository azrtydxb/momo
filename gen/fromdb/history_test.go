// SPDX-License-Identifier: Apache-2.0

package fromdb

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/schema"
)

// historySchema is an inspected database with a table and the audit history
// that c.History() builds for it.
func historySchema(t *testing.T) *schema.Schema {
	t.Helper()
	notes := schema.Table{
		Name: "notes",
		Columns: []schema.Column{
			{Name: "id", Type: "bigint"},
			{Name: "title", Type: "text"},
			{Name: "body", Type: "text", Nullable: true},
		},
		PrimaryKey: []string{"id"},
	}
	h, err := schema.HistoryFor(&notes)
	if err != nil {
		t.Fatal(err)
	}
	return &schema.Schema{
		FormatVersion: schema.FormatVersion,
		Dialect:       dialect.Postgres,
		Tables:        []schema.Table{notes, h.Table},
		Functions:     []schema.Function{h.Function},
		Triggers:      []schema.Trigger{h.Trigger},
	}
}

// Catches: a database with an audit history regenerating into models that
// declare it twice: the history table as a model of its own, the function and
// the trigger as c.Function and c.Trigger, and c.History() on top. The
// regenerated models would then fail on the duplicate objects, or silently
// differ from the database.
func TestGenerateFromDBRecognisesAnAuditHistory(t *testing.T) {
	s := historySchema(t)
	res, files := generate(t, s, Config{})
	if res.Failed() {
		t.Fatalf("findings: %v", res.Findings)
	}
	mustContain(t, "notes model", files["notes"], "c.History()")
	for _, bad := range []string{"c.Function(", "c.Trigger("} {
		if strings.Contains(files["notes"], bad) {
			t.Errorf("the history function or trigger is written next to c.History(): found %s in\n%s", bad, files["notes"])
		}
	}
	if _, ok := files["notes_history"]; ok || len(res.Files) != 1 {
		t.Errorf("the history table got a model of its own: %d files", len(res.Files))
	}
	if !schema.Equal(res.Snapshot, s) {
		t.Errorf("the snapshot is not the inspected schema: the models build the history objects, so it must hold them:\n%+v", res.Snapshot)
	}
	found := false
	for _, f := range res.Findings {
		found = found || (f.Severity == SevNote && strings.Contains(f.Msg, "audit history"))
	}
	if !found {
		t.Errorf("no note says the history was recognised: %v", res.Findings)
	}
}

// Catches: a hand-made table that merely looks like a history (an extra
// column, a different trigger) being swallowed into c.History(), which would
// rebuild it as another table and lose what the database has.
func TestGenerateFromDBKeepsALookalikeHistoryOrdinary(t *testing.T) {
	extra := schema.Column{Name: "note", Type: "text", Nullable: true}
	cases := []struct {
		name   string
		mutate func(*schema.Schema)
	}{
		{"an extra column", func(s *schema.Schema) { s.Tables[1].Columns = append(s.Tables[1].Columns, extra) }},
		{"another body", func(s *schema.Schema) { s.Functions[0].Body = "BEGIN RETURN NULL; END" }},
		{"another timing", func(s *schema.Schema) { s.Triggers[0].Timing = "BEFORE" }},
		{"another event set", func(s *schema.Schema) { s.Triggers[0].Events = []string{"UPDATE"} }},
	}
	for _, c := range cases {
		name := c.name
		s := historySchema(t)
		c.mutate(s)
		res, files := generate(t, s, Config{})
		if res.Failed() {
			t.Fatalf("%s: findings: %v", name, res.Findings)
		}
		if strings.Contains(files["notes"], "c.History()") {
			t.Errorf("%s: recognised as an audit history:\n%s", name, files["notes"])
		}
		if _, ok := files["notes_history"]; !ok {
			t.Errorf("%s: the lookalike has no model of its own", name)
		}
		mustContain(t, name+": notes model", files["notes"], "c.Trigger(", "c.Function(")
	}
}

// Catches: a recognised history on a dialect that has no c.History(), where
// the same-looking tables must stay ordinary.
func TestGenerateFromDBRecognisesHistoryOnPostgresOnly(t *testing.T) {
	s := historySchema(t)
	s.Dialect = dialect.SQLite
	s.Functions = nil
	s.Triggers = nil
	res, err := Generate(s, Options{Package: "models", Dialect: dialect.SQLite})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range res.Files {
		if strings.Contains(string(f.Content), "c.History()") {
			t.Errorf("c.History() written for SQLite:\n%s", f.Content)
		}
	}
}
