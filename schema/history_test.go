// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"bytes"
	"slices"
	"strings"
	"testing"
)

func auditedTable() Table {
	return Table{
		Name: "orders",
		Columns: []Column{
			{Name: "id", Type: "bigint", Identity: "always", Comment: "the key"},
			{Name: "status", Type: "text", Default: "'new'", Collate: "C"},
			{Name: "total", Type: "numeric(10,2)", Nullable: true},
			{Name: "slug", Type: "text", Generated: "lower(status)"},
			{Name: "we\"ird", Type: "integer"},
		},
		PrimaryKey: []string{"id"},
		Indexes:    []Index{{Name: "orders_status_idx", Parts: []IndexPart{{Column: "status"}}}},
		Checks:     []Check{{Name: "orders_total_check", Expr: "total >= 0"}},
	}
}

// Catches: a history table that copies what must not be copied (identity,
// generated expressions, constraints, indexes: a copied unique constraint
// would reject the second version of a row) or that loses what a column
// needs (type, nullability, default).
func TestHistoryForCopiesColumnsNotConstraints(t *testing.T) {
	tbl := auditedTable()
	h, err := HistoryFor(&tbl)
	if err != nil {
		t.Fatal(err)
	}
	if h.Table.Name != "orders_history" || !slices.Equal(h.Table.PrimaryKey, []string{"history_id"}) {
		t.Errorf("table = %q with key %v", h.Table.Name, h.Table.PrimaryKey)
	}
	var names []string
	for _, c := range h.Table.Columns {
		names = append(names, c.Name)
	}
	want := []string{"id", "status", "total", "slug", `we"ird`, "history_id", "history_operation", "history_changed_at", "history_changed_by"}
	if !slices.Equal(names, want) {
		t.Fatalf("columns = %v, want %v", names, want)
	}
	byName := map[string]Column{}
	for _, c := range h.Table.Columns {
		byName[c.Name] = c
	}
	if c := byName["id"]; c.Identity != "" || c.Comment != "" || c.Type != "bigint" {
		t.Errorf("id = %+v, want a plain bigint: the history repeats the key value, it does not generate it", c)
	}
	if c := byName["status"]; c.Default != "'new'" || c.Collate != "C" || c.Nullable {
		t.Errorf("status = %+v, want its default, collation and NOT NULL kept", c)
	}
	if c := byName["total"]; !c.Nullable || c.Type != "numeric(10,2)" {
		t.Errorf("total = %+v, want nullable numeric(10,2)", c)
	}
	if c := byName["slug"]; c.Generated != "" {
		t.Errorf("slug = %+v, want a plain column: a generated column cannot be written by the trigger", c)
	}
	if c := byName["history_id"]; c.Identity != "always" || c.Type != "bigint" || c.Nullable {
		t.Errorf("history_id = %+v, want a bigint identity", c)
	}
	if c := byName["history_changed_by"]; !c.Nullable || c.Type != "text" {
		t.Errorf("history_changed_by = %+v, want nullable text: a change made with no actor records NULL", c)
	}
	for _, name := range []string{"history_operation", "history_changed_at"} {
		if byName[name].Nullable {
			t.Errorf("%s is nullable", name)
		}
	}
	if len(h.Table.Uniques)+len(h.Table.Checks)+len(h.Table.ForeignKeys) != 0 {
		t.Errorf("the history table has constraints of the model: %+v", h.Table)
	}
	if len(h.Table.Indexes) != 1 || h.Table.Indexes[0].Name != "orders_history_row_idx" {
		t.Fatalf("indexes = %+v, want only orders_history_row_idx", h.Table.Indexes)
	}
	var parts []string
	for _, p := range h.Table.Indexes[0].Parts {
		parts = append(parts, p.Column)
	}
	if !slices.Equal(parts, []string{"id", "history_changed_at", "history_id"}) {
		t.Errorf("the row index covers %v, want the key, then time, then history id", parts)
	}
}

// Catches: a trigger that fires on the wrong events or the wrong side of the
// change, a function that records the wrong row for a delete, one that does
// not quote a hostile column name, and a trigger the PostgreSQL inspector
// would read back differently (the function is spelled name()).
func TestHistoryForTriggerAndFunction(t *testing.T) {
	tbl := auditedTable()
	h, err := HistoryFor(&tbl)
	if err != nil {
		t.Fatal(err)
	}
	trg := h.Trigger
	if trg.Name != "orders_history_trg" || trg.Table != "orders" || trg.Timing != "AFTER" || !trg.ForEachRow || trg.Function != "orders_history_fn()" ||
		!slices.Equal(trg.Events, []string{"DELETE", "INSERT", "UPDATE"}) {
		t.Errorf("trigger = %+v", trg)
	}
	fn := h.Function
	if fn.Name != "orders_history_fn" || fn.Returns != "trigger" || fn.Language != "plpgsql" || fn.Args != "" || fn.Volatility != "VOLATILE" {
		t.Errorf("function = %+v", fn)
	}
	for _, want := range []string{
		`IF TG_OP = 'DELETE' THEN`,
		`INSERT INTO "orders_history" ("id", "status", "total", "slug", "we""ird", "history_operation", "history_changed_at", "history_changed_by") VALUES (OLD."id", OLD."status", OLD."total", OLD."slug", OLD."we""ird", TG_OP, clock_timestamp(), nullif(current_setting('momo.actor', true), ''));`,
		`VALUES (NEW."id", NEW."status", NEW."total", NEW."slug", NEW."we""ird", TG_OP,`,
		`RETURN OLD;`,
		`RETURN NEW;`,
	} {
		if !strings.Contains(fn.Body, want) {
			t.Errorf("the function body lacks %s:\n%s", want, fn.Body)
		}
	}
}

// Catches: a model column change that does not change the function body, so
// Diff would not rewrite the function and the trigger would insert a column
// that is gone.
func TestHistoryForFollowsTheColumns(t *testing.T) {
	a := auditedTable()
	b := auditedTable()
	b.Columns = append(b.Columns, Column{Name: "note", Type: "text", Nullable: true})
	ha, err := HistoryFor(&a)
	if err != nil {
		t.Fatal(err)
	}
	hb, err := HistoryFor(&b)
	if err != nil {
		t.Fatal(err)
	}
	if ha.Function.Body == hb.Function.Body || len(hb.Table.Columns) != len(ha.Table.Columns)+1 {
		t.Errorf("a new model column left the history objects as they were")
	}
}

// Catches: a table that cannot have a history being given a broken one: the
// error must say why instead of building objects PostgreSQL would reject or
// silently rename.
func TestHistoryForRefusals(t *testing.T) {
	long := auditedTable()
	long.Name = strings.Repeat("x", 48)
	noKey := auditedTable()
	noKey.PrimaryKey = nil
	scoped := auditedTable()
	scoped.Schema = "app"
	clash := auditedTable()
	clash.Columns = append(clash.Columns, Column{Name: "history_changed_by", Type: "text"})
	for name, c := range map[string]struct {
		t    Table
		want string
	}{
		"no key":   {noKey, "no primary key"},
		"schema":   {scoped, `in the schema "app"`},
		"clash":    {clash, `column named "history_changed_by"`},
		"too long": {long, "longer than 63 bytes"},
	} {
		if _, err := HistoryFor(&c.t); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: error = %v, want one mentioning %q", name, err, c.want)
		}
	}
	ok := auditedTable()
	ok.Name = strings.Repeat("x", 47)
	if _, err := HistoryFor(&ok); err != nil {
		t.Errorf("a 47 byte table name: %v, want the longest allowed one accepted", err)
	}
}

// Catches: history objects that a snapshot loses or changes in a JSON round
// trip, which would make every later Diff against the snapshot propose to
// recreate them.
func TestHistoryObjectsSurviveASnapshotRoundTrip(t *testing.T) {
	tbl := auditedTable()
	h, err := HistoryFor(&tbl)
	if err != nil {
		t.Fatal(err)
	}
	in := &Schema{Dialect: "postgres", Tables: []Table{tbl, h.Table}, Functions: []Function{h.Function}, Triggers: []Trigger{h.Trigger}}
	var buf bytes.Buffer
	if err := in.Write(&buf); err != nil {
		t.Fatal(err)
	}
	out, err := Load(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if !Equal(in, out) {
		t.Errorf("the snapshot changed in a round trip:\n%+v\n%+v", in, out)
	}
	if in.FormatVersion != 1 {
		t.Errorf("format version = %d, want 1: history adds no snapshot field", in.FormatVersion)
	}
}
