// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"errors"
	"slices"
	"testing"

	"github.com/azrtydxb/momo/schema"
)

// notesWithHistory is the schema the generator builds for a notes model with
// Config.History: the table and the objects schema.HistoryFor derives from it.
func notesWithHistory(cols ...schema.Column) *schema.Schema {
	t := schema.Table{Name: "notes", Columns: cols, PrimaryKey: []string{"id"}}
	h, err := schema.HistoryFor(&t)
	if err != nil {
		panic(err)
	}
	return &schema.Schema{Dialect: "postgres", Tables: []schema.Table{t, h.Table}, Functions: []schema.Function{h.Function}, Triggers: []schema.Trigger{h.Trigger}}
}

// notesPlain is the same table with no history.
func notesPlain(cols ...schema.Column) *schema.Schema {
	return &schema.Schema{Dialect: "postgres", Tables: []schema.Table{{Name: "notes", Columns: cols, PrimaryKey: []string{"id"}}}}
}

func historyBase() []schema.Column {
	return []schema.Column{{Name: "id", Type: "bigint"}, {Name: "title", Type: "text"}, {Name: "body", Type: "text", Nullable: true}}
}

// described lists the changes as "kind table name", sorted.
func described(cs []Change) []string {
	var out []string
	for _, c := range cs {
		out = append(out, string(c.Kind)+" "+c.Table+" "+c.Name)
	}
	slices.Sort(out)
	return out
}

// Catches: a column change on the model that Diff does not repeat on the
// history table, so the trigger function inserts a column the history table
// lacks (or keeps writing one that was dropped) and every later write fails.
func TestDiffMirrorsModelColumnChangesOnTheHistoryTable(t *testing.T) {
	cases := []struct {
		name string
		to   []schema.Column
		want []string
	}{
		{
			"add and drop",
			[]schema.Column{{Name: "id", Type: "bigint"}, {Name: "title", Type: "text"}, {Name: "rank", Type: "integer", Default: "0"}},
			[]string{
				"add_column notes rank", "add_column notes_history rank",
				"drop_column notes body", "drop_column notes_history body",
				"replace_function  notes_history_fn",
			},
		},
		{
			"alter",
			[]schema.Column{{Name: "id", Type: "bigint"}, {Name: "title", Type: "varchar(100)"}, {Name: "body", Type: "text", Nullable: true}},
			[]string{"alter_column notes title", "alter_column notes_history title"},
		},
	}
	for _, c := range cases {
		changes, _, err := Diff(notesWithHistory(historyBase()...), notesWithHistory(c.to...), DiffOptions{AllowDestructive: true})
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		got := described(changes)
		want := slices.Clone(c.want)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Errorf("%s: changes\n got %q\nwant %q", c.name, got, want)
		}
	}
}

// Catches: a no-change Diff that proposes work for the history objects, which
// would make every migrate diff write a migration.
func TestDiffOfUnchangedHistoryIsEmpty(t *testing.T) {
	changes, _, err := Diff(notesWithHistory(historyBase()...), notesWithHistory(historyBase()...), DiffOptions{})
	if err != nil || len(changes) != 0 {
		t.Fatalf("diff of identical schemas = %v, %v, want nothing", described(changes), err)
	}
}

// Catches: turning history on creating the trigger before its function or its
// table exists, or leaving one of the three objects out.
func TestDiffCreatesTheHistoryObjectsInDependencyOrder(t *testing.T) {
	changes, _, err := Diff(notesPlain(historyBase()...), notesWithHistory(historyBase()...), DiffOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var order []ChangeKind
	for _, c := range changes {
		order = append(order, c.Kind)
	}
	tableAt := slices.Index(order, CreateTable)
	fnAt := slices.Index(order, CreateFunction)
	trgAt := slices.Index(order, CreateTrigger)
	if tableAt < 0 || fnAt < 0 || trgAt < 0 || trgAt < tableAt || trgAt < fnAt {
		t.Fatalf("changes = %v, want create_table and create_function before create_trigger", order)
	}
	if got := described(changes); !slices.Equal(got, []string{"create_function  notes_history_fn", "create_table notes_history notes_history", "create_trigger notes notes_history_trg"}) {
		t.Errorf("changes = %q", got)
	}
}

// Catches: turning history off silently dropping the recorded versions: the
// history table is a destructive drop that needs --allow-destructive, like any
// other table.
func TestDiffRefusesToDropTheHistoryTableUnasked(t *testing.T) {
	_, _, err := Diff(notesWithHistory(historyBase()...), notesPlain(historyBase()...), DiffOptions{})
	if !errors.Is(err, ErrDestructive) {
		t.Fatalf("removing the history without AllowDestructive: %v, want ErrDestructive", err)
	}
	changes, _, err := Diff(notesWithHistory(historyBase()...), notesPlain(historyBase()...), DiffOptions{AllowDestructive: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := described(changes); !slices.Equal(got, []string{"drop_function  notes_history_fn", "drop_table notes_history notes_history", "drop_trigger notes notes_history_trg"}) {
		t.Errorf("changes = %q", got)
	}
}
