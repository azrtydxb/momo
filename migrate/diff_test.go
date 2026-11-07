// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/schema"
)

func base() *schema.Schema {
	return &schema.Schema{Dialect: "postgres", Tables: []schema.Table{{
		Name:       "users",
		Columns:    []schema.Column{{Name: "id", Type: "bigint"}, {Name: "email", Type: "text"}},
		PrimaryKey: []string{"id"},
	}}}
}

func kinds(cs []Change) []ChangeKind {
	var out []ChangeKind
	for _, c := range cs {
		out = append(out, c.Kind)
	}
	return out
}

func TestNoRenameGuessing(t *testing.T) {
	from := base()
	to := base()
	to.Tables[0].Columns[1] = schema.Column{Name: "mail", Type: "text", Nullable: true}
	changes, hints, err := Diff(from, to, DiffOptions{AllowDestructive: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range changes {
		if c.Kind == "rename_column" {
			t.Fatalf("diff guessed a rename: %+v", c)
		}
	}
	if len(hints) == 0 {
		t.Fatal("expected a rename hint for drop+add of the same type")
	}
	want := "possible rename: drop+add of users.email -> users.mail; pass --rename-column users.email=mail to rename"
	if hints[0] != want {
		t.Fatalf("hint = %q, want %q", hints[0], want)
	}
}

func TestExplicitRenames(t *testing.T) {
	from := base()
	to := base()
	to.Tables[0].Columns[1] = schema.Column{Name: "mail", Type: "text"}
	changes, hints, err := Diff(from, to, DiffOptions{RenameColumns: map[string]string{"users.email": "mail"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := kinds(changes); !reflect.DeepEqual(got, []ChangeKind{RenameColumn}) || changes[0].Destructive {
		t.Fatalf("want one non-destructive rename_column, got %+v", changes)
	}
	if len(hints) != 0 {
		t.Fatalf("unexpected hints %v", hints)
	}

	to2 := base()
	to2.Tables[0].Name = "people"
	changes, _, err = Diff(from, to2, DiffOptions{RenameTables: map[string]string{"users": "people"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := kinds(changes); !reflect.DeepEqual(got, []ChangeKind{RenameTable}) || changes[0].Name != "people" {
		t.Fatalf("want one rename_table, got %+v", changes)
	}

	if _, _, err := Diff(from, base(), DiffOptions{RenameColumns: map[string]string{"users.nope": "x"}}); err == nil {
		t.Fatal("a rename flag that matches nothing must be an error")
	}
}

func TestDestructiveRequiresFlag(t *testing.T) {
	from := base()
	to := base()
	to.Tables[0].Columns = to.Tables[0].Columns[:1]
	_, _, err := Diff(from, to, DiffOptions{})
	if !errors.Is(err, ErrDestructive) {
		t.Fatalf("want ErrDestructive, got %v", err)
	}
	changes, _, err := Diff(from, to, DiffOptions{AllowDestructive: true})
	if err != nil {
		t.Fatalf("with the flag the diff must succeed: %v", err)
	}
	if len(changes) != 1 || changes[0].Kind != DropColumn || !changes[0].Destructive {
		t.Fatalf("want one destructive drop_column, got %+v", changes)
	}
}

func TestDestructiveTypeChangeAndDropTable(t *testing.T) {
	from := base()
	to := base()
	to.Tables[0].Columns[1].Type = "integer"
	if _, _, err := Diff(from, to, DiffOptions{}); !errors.Is(err, ErrDestructive) {
		t.Fatalf("type change must be destructive, got %v", err)
	}
	empty := &schema.Schema{Dialect: "postgres"}
	if _, _, err := Diff(from, empty, DiffOptions{}); !errors.Is(err, ErrDestructive) {
		t.Fatalf("drop_table must be destructive, got %v", err)
	}
}

func TestNormalizedTypesAreNoDiff(t *testing.T) {
	from := base()
	from.Tables[0].Columns[1].Type = "varchar(10)"
	to := base()
	to.Tables[0].Columns[1].Type = "character varying(10)"
	changes, hints, err := Diff(from, to, DiffOptions{})
	if err != nil || len(changes) != 0 || len(hints) != 0 {
		t.Fatalf("spelling differences are no diff: %v %v %v", changes, hints, err)
	}
}

func TestStagedChangeDiff(t *testing.T) {
	from := base()
	to := base()
	to.Tables[0].Columns = append(to.Tables[0].Columns, schema.Column{Name: "tenant", Type: "bigint"})
	_, _, err := Diff(from, to, DiffOptions{})
	if !errors.Is(err, ErrStagedRequired) {
		t.Fatalf("NOT NULL without default must need staging, got %v", err)
	}
	changes, hints, err := Diff(from, to, DiffOptions{Stage: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Kind != "add_column" || changes[0].After.(*schema.Column).Nullable != true {
		t.Fatalf("staged add must be nullable: %+v", changes)
	}
	if len(hints) == 0 {
		t.Fatal("staged add must hint the backfill and SET NOT NULL steps")
	}
	if to.Tables[0].Columns[2].Nullable {
		t.Fatal("Diff must not modify its input")
	}
}

func TestStagedNotNullOnExistingColumn(t *testing.T) {
	from := base()
	from.Tables[0].Columns[1].Nullable = true
	to := base()
	if _, _, err := Diff(from, to, DiffOptions{}); !errors.Is(err, ErrStagedRequired) {
		t.Fatalf("NULL to NOT NULL needs staging, got %v", err)
	}
	changes, hints, err := Diff(from, to, DiffOptions{Stage: true})
	if err != nil || len(changes) != 0 || len(hints) == 0 {
		t.Fatalf("staged NULL to NOT NULL emits only a hint: %v %v %v", changes, hints, err)
	}
	to.Tables[0].Columns[1].Default = "''"
	changes, _, err = Diff(from, to, DiffOptions{})
	if err != nil || len(changes) != 1 || changes[0].Kind != AlterColumn {
		t.Fatalf("with a default no staging is needed: %v %v", changes, err)
	}
}

func TestNonDestructiveAlter(t *testing.T) {
	from := base()
	to := base()
	to.Tables[0].Columns[1].Nullable = true
	to.Tables[0].Columns[1].Default = "'x'"
	changes, _, err := Diff(from, to, DiffOptions{})
	if err != nil || len(changes) != 1 || changes[0].Kind != AlterColumn || changes[0].Destructive {
		t.Fatalf("want one non-destructive alter_column: %v %v", changes, err)
	}
}

func TestCollateCommentDiff(t *testing.T) {
	for name, mut := range map[string]func(*schema.Column){
		"collate": func(c *schema.Column) { c.Collate = "C" },
		"comment": func(c *schema.Column) { c.Comment = "login address" },
	} {
		t.Run(name, func(t *testing.T) {
			from := base()
			to := base()
			mut(&to.Tables[0].Columns[1])
			changes, _, err := Diff(from, to, DiffOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if len(changes) != 1 || changes[0].Kind != AlterColumn || changes[0].Destructive {
				t.Fatalf("want one non-destructive alter_column, got %+v", changes)
			}
			// round trip: diffing the changed schema against itself is empty, and
			// reverting yields the inverse change.
			if cs, _, err := Diff(to, to, DiffOptions{}); err != nil || len(cs) != 0 {
				t.Fatalf("self diff: %v %v", cs, err)
			}
			back, _, err := Diff(to, from, DiffOptions{})
			if err != nil || len(back) != 1 || back[0].Kind != AlterColumn {
				t.Fatalf("revert: %v %v", back, err)
			}
		})
	}
}

func TestFKActions(t *testing.T) {
	from := base()
	to := base()
	to.Tables[0].ForeignKeys = []schema.ForeignKey{{Name: "fk", Columns: []string{"id"}, RefTable: "orgs", RefColumns: []string{"id"}, OnDelete: "CASCADE"}}
	changes, _, err := Diff(from, to, DiffOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Kind != "add_fk" {
		t.Fatalf("want one add_fk, got %+v", changes)
	}
	if fk := changes[0].After.(*schema.ForeignKey); fk.OnDelete != "CASCADE" {
		t.Fatalf("action lost: %+v", fk)
	}

	// An action change is drop_fk then add_fk, for each of the two actions.
	for _, mut := range []func(*schema.ForeignKey){
		func(f *schema.ForeignKey) { f.OnDelete = "SET NULL" },
		func(f *schema.ForeignKey) { f.OnUpdate = "CASCADE" },
	} {
		to2 := base()
		fk := to.Tables[0].ForeignKeys[0]
		mut(&fk)
		to2.Tables[0].ForeignKeys = []schema.ForeignKey{fk}
		changes, _, err = Diff(to, to2, DiffOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if got := kinds(changes); !reflect.DeepEqual(got, []ChangeKind{DropFK, AddFK}) || changes[0].Destructive {
			t.Fatalf("want drop_fk, add_fk (not destructive), got %+v", changes)
		}
	}

	// NO ACTION spelled out equals the empty default.
	to3 := base()
	fk := to.Tables[0].ForeignKeys[0]
	fk.OnDelete = ""
	to3.Tables[0].ForeignKeys = []schema.ForeignKey{fk}
	to4 := base()
	fk.OnDelete, fk.OnUpdate = "no action", "NO ACTION"
	to4.Tables[0].ForeignKeys = []schema.ForeignKey{fk}
	if cs, _, err := Diff(to3, to4, DiffOptions{}); err != nil || len(cs) != 0 {
		t.Fatalf("NO ACTION is the default: %v %v", cs, err)
	}

	// Dropping an FK is allowed without the destructive flag.
	changes, _, err = Diff(to, base(), DiffOptions{})
	if err != nil || len(changes) != 1 || changes[0].Kind != DropFK {
		t.Fatalf("drop_fk: %v %v", changes, err)
	}
}

func TestDiffNoChangeEmpty(t *testing.T) {
	changes, hints, err := Diff(base(), base(), DiffOptions{})
	if err != nil || len(changes) != 0 || len(hints) != 0 {
		t.Fatalf("identical schemas must produce no changes: %v %v %v", changes, hints, err)
	}
}

func TestDiffCreateTableAndObjects(t *testing.T) {
	to := base()
	to.Tables = append(to.Tables, schema.Table{
		Name:    "orgs",
		Columns: []schema.Column{{Name: "id", Type: "bigint"}},
		Indexes: []schema.Index{{Name: "orgs_id", Parts: []schema.IndexPart{{Column: "id"}}}},
	})
	to.Enums = []schema.Enum{{Name: "mood", Values: []string{"ok"}}}
	to.Views = []schema.View{{Name: "v", Query: "SELECT 1"}}
	changes, _, err := Diff(base(), to, DiffOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want := []ChangeKind{CreateEnum, CreateTable, CreateIndex, CreateView}
	if got := kinds(changes); !reflect.DeepEqual(got, want) {
		t.Fatalf("kinds = %v, want %v", got, want)
	}
	again, _, _ := Diff(base(), to, DiffOptions{})
	if !reflect.DeepEqual(changes, again) {
		t.Fatal("diff output must be deterministic")
	}
	if _, _, err := Diff(to, base(), DiffOptions{}); !errors.Is(err, ErrDestructive) || !strings.Contains(err.Error(), "orgs") {
		t.Fatalf("destructive error must name the object, got %v", err)
	}
}

func TestDiffFunctionViewChanges(t *testing.T) {
	from := base()
	from.Views = []schema.View{{Name: "v", Query: "select 1"}}
	from.Functions = []schema.Function{{Name: "f", Args: "", Returns: "integer", Language: "sql", Body: "select 1"}}
	to := base()
	to.Views = []schema.View{{Name: "v", Query: "select 2"}}
	to.Functions = []schema.Function{{Name: "f", Args: "", Returns: "integer", Language: "sql", Body: "select 2"}}
	changes, _, err := Diff(from, to, DiffOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want := []ChangeKind{DropView, CreateView, ReplaceFunction}
	if got := kinds(changes); !reflect.DeepEqual(got, want) {
		t.Fatalf("kinds = %v, want %v", got, want)
	}
	to.Views[0].Query = "SELECT   1"
	to.Functions[0].Body = "SELECT 1"
	if cs, _, err := Diff(from, to, DiffOptions{}); err != nil || len(cs) != 0 {
		t.Fatalf("normalized equal queries are no diff: %v %v", cs, err)
	}
}

func TestDiffTriggerPolicyChanges(t *testing.T) {
	from := base()
	from.Triggers = []schema.Trigger{
		{Table: "users", Name: "t_same", Timing: "BEFORE", Events: []string{"INSERT"}, Function: "f"},
		{Table: "users", Name: "t_changed", Timing: "BEFORE", Events: []string{"INSERT"}, Function: "f"},
		{Table: "users", Name: "t_gone", Timing: "AFTER", Events: []string{"DELETE"}, Function: "f"},
	}
	from.Policies = []schema.Policy{
		{Table: "users", Name: "p_same", Command: "ALL", Permissive: true, Using: "true"},
		{Table: "users", Name: "p_changed", Command: "ALL", Permissive: true, Using: "true"},
		{Table: "users", Name: "p_gone", Command: "SELECT", Permissive: true, Using: "true"},
	}
	from.Functions = []schema.Function{{Name: "f_gone", Returns: "integer", Language: "sql", Body: "select 1"}}
	to := base()
	to.Triggers = []schema.Trigger{
		{Table: "users", Name: "t_same", Timing: "BEFORE", Events: []string{"INSERT"}, Function: "f"},
		{Table: "users", Name: "t_changed", Timing: "AFTER", Events: []string{"INSERT"}, Function: "f"},
		{Table: "users", Name: "t_new", Timing: "AFTER", Events: []string{"UPDATE"}, Function: "f"},
	}
	to.Policies = []schema.Policy{
		{Table: "users", Name: "p_same", Command: "ALL", Permissive: true, Using: "true"},
		{Table: "users", Name: "p_changed", Command: "ALL", Permissive: true, Using: "false"},
		{Table: "users", Name: "p_new", Command: "SELECT", Permissive: true, Using: "true"},
	}
	to.Functions = []schema.Function{{Name: "f_new", Returns: "integer", Language: "sql", Body: "select 1"}}
	changes, _, err := Diff(from, to, DiffOptions{AllowDestructive: true})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	for _, c := range changes {
		switch c.Kind {
		case CreateTrigger, DropTrigger, CreatePolicy, DropPolicy, CreateFunction, DropFunction:
			got[string(c.Kind)+" "+c.Table+"."+c.Name]++
		}
	}
	want := map[string]int{
		"create_trigger users.t_changed": 1, "drop_trigger users.t_changed": 1,
		"create_trigger users.t_new": 1, "drop_trigger users.t_gone": 1,
		"create_policy users.p_changed": 1, "drop_policy users.p_changed": 1,
		"create_policy users.p_new": 1, "drop_policy users.p_gone": 1,
		"create_function .f_new": 1, "drop_function .f_gone": 1,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("changes = %v, want %v", got, want)
	}
}
