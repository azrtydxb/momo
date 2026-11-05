// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"slices"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/schema"
)

// viewDep is a test stand-in for a managed object that reports dependencies.
type viewDep struct{ Depends []string }

func (v viewDep) dependsOn() []string { return v.Depends }

func TestDropViewBeforeDependentView(t *testing.T) {
	changes := []Change{
		{Kind: "drop_view", Name: "base_v", After: nil, Before: viewDep{Depends: nil}},
		{Kind: "drop_view", Name: "top_v", Before: viewDep{Depends: []string{"base_v"}}},
	}
	got := Order(changes)
	if got[0].Name != "top_v" || got[1].Name != "base_v" {
		t.Fatalf("dependent view must drop first, got %s then %s", got[0].Name, got[1].Name)
	}
}

func TestCreateViewAfterDependency(t *testing.T) {
	changes := []Change{
		{Kind: "create_view", Name: "top_v", After: viewDep{Depends: []string{"base_v"}}},
		{Kind: "create_view", Name: "base_v", After: viewDep{}},
	}
	got := Order(changes)
	if got[0].Name != "base_v" {
		t.Fatalf("dependency must be created first, got %s", got[0].Name)
	}
}

func TestCycleIsAnError(t *testing.T) {
	changes := []Change{
		{Kind: "create_view", Name: "a", After: viewDep{Depends: []string{"b"}}},
		{Kind: "create_view", Name: "b", After: viewDep{Depends: []string{"a"}}},
	}
	_, err := OrderErr(changes)
	if err == nil {
		t.Fatal("cycle must be reported, not silently ordered")
	}
	if !strings.Contains(err.Error(), "a") || !strings.Contains(err.Error(), "b") {
		t.Fatalf("error must name the objects: %v", err)
	}
}

func names(cs []Change) []string {
	var out []string
	for _, c := range cs {
		out = append(out, string(c.Kind)+":"+c.Name)
	}
	return out
}

func idx(t *testing.T, cs []Change, want string) int {
	t.Helper()
	i := slices.Index(names(cs), want)
	if i < 0 {
		t.Fatalf("%s missing from %v", want, names(cs))
	}
	return i
}

func ctable(name string, fks ...schema.ForeignKey) Change {
	return Change{Kind: CreateTable, Name: name, Table: name, After: &schema.Table{Name: name, ForeignKeys: fks}}
}

func TestOrderCreateTablesByFK(t *testing.T) {
	got := Order([]Change{
		ctable("child", schema.ForeignKey{Name: "fk", RefTable: "parent"}),
		ctable("parent"),
	})
	if idx(t, got, "create_table:parent") > idx(t, got, "create_table:child") {
		t.Fatalf("referenced table first: %v", names(got))
	}
}

func TestOrderCyclicCreateSplitsFK(t *testing.T) {
	got, err := OrderErr([]Change{
		ctable("a", schema.ForeignKey{Name: "a_b", RefTable: "b"}),
		ctable("b", schema.ForeignKey{Name: "b_a", RefTable: "a"}),
		ctable("self", schema.ForeignKey{Name: "self_self", RefTable: "self"}),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range got {
		if c.Kind == CreateTable {
			tb := c.After.(*schema.Table)
			switch c.Name {
			case "self":
				if len(tb.ForeignKeys) != 1 {
					t.Fatalf("self FK stays inline")
				}
			default:
				if len(tb.ForeignKeys) != 0 {
					t.Fatalf("%s keeps cyclic FK inline", c.Name)
				}
			}
		}
	}
	last := max(idx(t, got, "create_table:a"), idx(t, got, "create_table:b"))
	for _, n := range []string{"add_fk:a_b", "add_fk:b_a"} {
		if idx(t, got, n) < last {
			t.Fatalf("%s must follow all create_table: %v", n, names(got))
		}
	}
}

func TestOrderCyclicDropDropsFKsFirst(t *testing.T) {
	got, err := OrderErr([]Change{
		{Kind: DropTable, Name: "a", Table: "a", Before: &schema.Table{Name: "a"}},
		{Kind: DropTable, Name: "b", Table: "b", Before: &schema.Table{Name: "b"}},
		{Kind: DropFK, Table: "a", Name: "a_b"},
		{Kind: DropFK, Table: "b", Name: "b_a"},
	})
	if err != nil {
		t.Fatal(err)
	}
	first := min(idx(t, got, "drop_table:a"), idx(t, got, "drop_table:b"))
	if idx(t, got, "drop_fk:a_b") > first || idx(t, got, "drop_fk:b_a") > first {
		t.Fatalf("FKs before tables: %v", names(got))
	}
}

func TestOrderPartitions(t *testing.T) {
	part := func(n, parent string) *schema.Table {
		return &schema.Table{Name: n, PartitionOf: &schema.PartitionOf{Parent: parent}}
	}
	got := Order([]Change{
		{Kind: CreateTable, Name: "p1", After: part("p1", "p")},
		{Kind: CreateTable, Name: "p", After: &schema.Table{Name: "p"}},
	})
	if idx(t, got, "create_table:p") > idx(t, got, "create_table:p1") {
		t.Fatalf("parent first on create: %v", names(got))
	}
	got = Order([]Change{
		{Kind: DropTable, Name: "p", Before: &schema.Table{Name: "p"}},
		{Kind: DropTable, Name: "p1", Before: part("p1", "p")},
	})
	if idx(t, got, "drop_table:p1") > idx(t, got, "drop_table:p") {
		t.Fatalf("child first on drop: %v", names(got))
	}
}

func TestOrderPhases(t *testing.T) {
	got := Order([]Change{
		{Kind: DropEnum, Name: "old"},
		{Kind: CreateTrigger, Name: "trg"},
		{Kind: CreateView, Name: "v", After: &schema.View{Name: "v", Depends: []string{"t"}}},
		{Kind: CreateFunction, Name: "fn"},
		ctable("t"),
		{Kind: CreateEnum, Name: "e"},
		{Kind: CreateExtension, Name: "x"},
		{Kind: CreateSchema, Name: "s"},
		{Kind: DropFunction, Name: "oldfn"},
		{Kind: DropTrigger, Name: "oldtrg"},
		{Kind: RenameTable, Name: "r"},
	})
	want := []string{"rename_table:r", "drop_trigger:oldtrg", "create_schema:s", "create_extension:x", "create_enum:e",
		"create_table:t", "create_function:fn", "create_view:v", "create_trigger:trg", "drop_function:oldfn", "drop_enum:old"}
	if !slices.Equal(names(got), want) {
		t.Fatalf("got  %v\nwant %v", names(got), want)
	}
}

func TestOrderDeterministicAndPure(t *testing.T) {
	in := []Change{
		{Kind: CreateView, Name: "y", After: viewDep{}},
		{Kind: CreateView, Name: "x", After: viewDep{}},
	}
	cp1 := slices.Clone(in)
	a, b := Order(in), Order(in)
	if !slices.Equal(names(a), names(b)) || names(a)[0] != "create_view:y" {
		t.Fatalf("stable tie-break by input order: %v", names(a))
	}
	if !slices.Equal(names(in), names(cp1)) {
		t.Fatal("input mutated")
	}
}

func TestOrderQualifiedViewDeps(t *testing.T) {
	got := Order([]Change{
		{Kind: CreateView, Schema: "s", Name: "top", After: &schema.View{Schema: "s", Name: "top", Depends: []string{"s.base"}}},
		{Kind: CreateView, Schema: "s", Name: "base", After: &schema.View{Schema: "s", Name: "base"}},
	})
	if got[0].Name != "base" {
		t.Fatalf("qualified dependency first: %v", names(got))
	}
}

func TestOrderUnbreakableCycleReturnsInputUnchanged(t *testing.T) {
	in := []Change{
		{Kind: CreateView, Name: "a", After: viewDep{Depends: []string{"b"}}},
		{Kind: CreateView, Name: "b", After: viewDep{Depends: []string{"a"}}},
	}
	if got := Order(in); !slices.Equal(names(got), names(in)) {
		t.Fatalf("Order returns input on error: %v", names(got))
	}
}

func TestOrderWithViewsRebuildsTransitiveDependents(t *testing.T) {
	from := []schema.View{
		{Name: "base", Query: "select 1"},
		{Name: "mid", Query: "select * from base", Depends: []string{"base"}},
		{Name: "top", Query: "select * from mid", Depends: []string{"mid"}},
		{Name: "other", Query: "select 2"},
	}
	to := slices.Clone(from)
	to[0].Query = "select 3"
	in := []Change{
		{Kind: DropView, Name: "base", Before: &from[0]},
		{Kind: CreateView, Name: "base", After: &to[0]},
	}
	got, err := OrderWithViews(in, from, to)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"drop_view:top", "drop_view:mid", "drop_view:base", "create_view:base", "create_view:mid", "create_view:top"}
	if !slices.Equal(names(got), want) {
		t.Fatalf("got  %v\nwant %v", names(got), want)
	}
	if len(in) != 2 {
		t.Fatal("input mutated")
	}
}

func TestOrderWithViewsDroppedBaseDropsDependentOnce(t *testing.T) {
	from := []schema.View{{Name: "base"}, {Name: "top", Depends: []string{"base"}}}
	to := []schema.View{{Name: "top", Depends: []string{"base"}}}
	got, err := OrderWithViews([]Change{{Kind: DropView, Name: "base", Before: &from[0]}}, from, to)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"drop_view:top", "drop_view:base", "create_view:top"}
	if !slices.Equal(names(got), want) {
		t.Fatalf("got %v want %v", names(got), want)
	}
}

func TestOrderPairwisePhases(t *testing.T) {
	dt := Change{Kind: DropTable, Name: "t", Before: &schema.Table{Name: "t"}}
	cycA := ctable("ca", schema.ForeignKey{Name: "ca_cb", RefTable: "cb"})
	cycB := ctable("cb", schema.ForeignKey{Name: "cb_ca", RefTable: "ca"})
	cases := []struct {
		name          string
		in            []Change
		before, after string
	}{
		{"drop_view before drop_column", []Change{{Kind: DropColumn, Name: "c"}, {Kind: DropView, Name: "v"}}, "drop_view:v", "drop_column:c"},
		{"drop_fk before drop_table", []Change{dt, {Kind: DropFK, Name: "f"}}, "drop_fk:f", "drop_table:t"},
		{"drop_policy before drop_view", []Change{{Kind: DropView, Name: "v"}, {Kind: DropPolicy, Name: "p"}}, "drop_policy:p", "drop_view:v"},
		{"drop_policy before drop_table", []Change{dt, {Kind: DropPolicy, Name: "p"}}, "drop_policy:p", "drop_table:t"},
		{"rename before add_fk", []Change{{Kind: AddFK, Table: "new", Name: "f"}, {Kind: RenameTable, Table: "old", Name: "new"}}, "rename_table:new", "add_fk:f"},
		{"add_unique before add_fk", []Change{{Kind: AddFK, Name: "f"}, {Kind: AddUnique, Name: "u"}}, "add_unique:u", "add_fk:f"},
		{"drop_function after drop_table", []Change{{Kind: DropFunction, Name: "fn"}, dt}, "drop_table:t", "drop_function:fn"},
		{"drop_enum after drop_table", []Change{{Kind: DropEnum, Name: "e"}, dt}, "drop_table:t", "drop_enum:e"},
		{"drop_enum after alter_column", []Change{{Kind: DropEnum, Name: "e"}, {Kind: AlterColumn, Name: "c"}}, "alter_column:c", "drop_enum:e"},
		{"deferred add_fk after create_index", []Change{cycA, cycB, {Kind: CreateIndex, Name: "ix"}}, "create_index:ix", "add_fk:ca_cb"},
		{"add_fk after create_index", []Change{{Kind: AddFK, Name: "f"}, {Kind: CreateIndex, Name: "ix"}}, "create_index:ix", "add_fk:f"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := OrderErr(tc.in)
			if err != nil {
				t.Fatal(err)
			}
			if idx(t, got, tc.before) >= idx(t, got, tc.after) {
				t.Fatalf("%s must precede %s: %v", tc.before, tc.after, names(got))
			}
		})
	}
}
