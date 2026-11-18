// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/schema"
)

func changeKinds(cs []Change) []ChangeKind {
	var out []ChangeKind
	for _, c := range cs {
		out = append(out, c.Kind)
	}
	return out
}

func withheldKinds(ws []Skipped) []ChangeKind {
	var out []ChangeKind
	for _, w := range ws {
		out = append(out, w.Change.Kind)
	}
	return out
}

// Catches: a changed view, trigger, policy, index or constraint (a drop and a
// create of one name) printing the create while the drop is withheld, which
// fails with "already exists" and, on MySQL, leaves the earlier steps applied.
func TestSplitDestructiveWithholdsTheCreateThatReplacesADrop(t *testing.T) {
	e := &Engine{}
	add := Change{Kind: AddColumn, Table: "t", Name: "extra", After: &schema.Column{Name: "extra", Type: "int"}}
	changes := []Change{
		{Kind: DropView, Name: "v"}, {Kind: CreateView, Name: "v"},
		{Kind: DropTrigger, Table: "t", Name: "trg"}, {Kind: CreateTrigger, Table: "t", Name: "trg"},
		{Kind: DropPolicy, Table: "t", Name: "pol"}, {Kind: CreatePolicy, Table: "t", Name: "pol"},
		{Kind: DropIndex, Table: "t", Name: "ix"}, {Kind: CreateIndex, Table: "t", Name: "ix"},
		{Kind: DropCheck, Table: "t", Name: "ck"}, {Kind: AddCheck, Table: "t", Name: "ck"},
		{Kind: DropCheck, Table: "t", Name: "ci"}, {Kind: AlterCheckIn, Table: "t", Name: "ci"},
		{Kind: DropUnique, Table: "t", Name: "uq"}, {Kind: AddUnique, Table: "t", Name: "uq"},
		{Kind: DropFK, Table: "t", Name: "fk"}, {Kind: AddFK, Table: "t", Name: "fk"},
		{Kind: DropPrimaryKey, Table: "t", Name: "t_pkey"}, {Kind: AddPrimaryKey, Table: "t", Name: "t_pkey"},
		// A unique constraint that becomes an index of the same name, and the reverse.
		{Kind: DropUnique, Table: "t", Name: "ux"}, {Kind: CreateIndex, Table: "t", Name: "ux"},
		{Kind: DropIndex, Table: "t", Name: "iu"}, {Kind: AddUnique, Table: "t", Name: "iu"},
		// An index name is schema-wide: the drop on one table collides with the create on another.
		{Kind: DropIndex, Table: "a", Name: "shared"}, {Kind: CreateIndex, Table: "b", Name: "shared"},
		add,
	}
	kept, withheld := e.splitDestructive(nil, changes, false)
	if !slices.Equal(changeKinds(kept), []ChangeKind{AddColumn}) {
		t.Fatalf("kept = %v, want only the add_column", changeKinds(kept))
	}
	if len(withheld) != len(changes)-1 {
		t.Fatalf("withheld %d of %d changes: %v", len(withheld), len(changes), withheldKinds(withheld))
	}
	for _, w := range withheld {
		if strings.HasPrefix(string(w.Change.Kind), "drop_") {
			continue
		}
		if !strings.Contains(w.Reason, "replaces drop_") {
			t.Errorf("%s: reason %q does not say which withheld drop it replaces", w.Change.Kind, w.Reason)
		}
	}
	kept, withheld = e.splitDestructive(nil, changes, true)
	if len(kept) != len(changes) || len(withheld) != 0 {
		t.Fatalf("with allow nothing is withheld: kept %d withheld %d", len(kept), len(withheld))
	}
}

// Catches: a withheld DropFunction f(int) also withholding an unrelated
// CreateFunction f(text), and the reverse: the same name with the same
// arguments must still be paired.
func TestSplitDestructiveTellsFunctionsApartByArguments(t *testing.T) {
	e := &Engine{}
	fn := func(args string) *schema.Function { return &schema.Function{Name: "f", Args: args} }
	changes := []Change{
		{Kind: DropFunction, Name: "f", Before: fn("a integer")},
		{Kind: CreateFunction, Name: "f", After: fn("a text")},
		{Kind: DropFunction, Name: "g", Before: &schema.Function{Name: "g", Args: "a integer"}},
		{Kind: CreateFunction, Name: "g", After: &schema.Function{Name: "g", Args: "a integer"}},
	}
	kept, withheld := e.splitDestructive(nil, changes, false)
	if len(kept) != 1 || kept[0].Kind != CreateFunction || kept[0].Name != "f" {
		t.Fatalf("kept = %+v, want only create_function f(text)", kept)
	}
	if len(withheld) != 3 {
		t.Fatalf("withheld %v, want both drops and the create of g", withheldKinds(withheld))
	}
}

// Catches: an index name without its table ("ix", as drift names an invalid
// index whose table it could not find) never matching a change, which made a
// complete repair report Complete() == false.
func TestCoversMatchesABareIndexName(t *testing.T) {
	ref := ObjectRef{Kind: "index", Name: "orphan_ix"}
	if !covers([]Change{{Kind: DropIndex, Table: "t", Name: "orphan_ix"}}, ref) {
		t.Fatal("a drop_index of that name covers the bare index reference")
	}
	if covers([]Change{{Kind: DropIndex, Table: "t", Name: "other"}}, ref) {
		t.Fatal("a change of another name must not cover it")
	}
}

// Catches: a column type change being printed without --allow-destructive, and
// a step that needs the changed column (an index on it) being printed after it.
func TestSplitDestructiveWithholdsATypeChangeAndItsDependents(t *testing.T) {
	e := &Engine{}
	typeChange := Change{Kind: AlterColumn, Table: "t", Name: "c", Destructive: true,
		Before: &schema.Column{Name: "c", Type: "bigint"}, After: &schema.Column{Name: "c", Type: "int"}}
	index := Change{Kind: CreateIndex, Table: "t", Name: "t_c_idx", After: &schema.Index{Name: "t_c_idx", Parts: []schema.IndexPart{{Column: "c"}}}}
	other := Change{Kind: AddColumn, Table: "t", Name: "d", After: &schema.Column{Name: "d", Type: "int", Nullable: true}}
	kept, withheld := e.splitDestructive(nil, []Change{typeChange, index, other}, false)
	if !slices.Equal(changeKinds(kept), []ChangeKind{CreateIndex, AddColumn}) {
		t.Fatalf("kept = %v", changeKinds(kept))
	}
	if len(withheld) != 1 || withheld[0].Change.Kind != AlterColumn || !strings.Contains(withheld[0].Reason, "type") {
		t.Fatalf("withheld = %+v, want the type change", withheld)
	}
	kept, withheld = skipDependents(kept, withheld)
	if !slices.Equal(changeKinds(kept), []ChangeKind{AddColumn}) || !slices.Equal(withheldKinds(withheld), []ChangeKind{AlterColumn, CreateIndex}) {
		t.Fatalf("kept %v withheld %v: the index on the changed column must be withheld too", changeKinds(kept), withheldKinds(withheld))
	}
	if !strings.Contains(withheld[1].Reason, "depends on") {
		t.Errorf("dependent reason %q", withheld[1].Reason)
	}
}

// renderStub is a dialect that renders everything except a view, as a dialect
// with no view support would.
type renderStub struct{}

func (renderStub) Dialect() *dialect.Dialect { return dialect.For(dialect.Postgres) }
func (renderStub) Inspect(context.Context, momo.PinnedConn, InspectOptions) (*schema.Schema, error) {
	return nil, nil
}
func (renderStub) HistoryDDL() []string { return nil }
func (renderStub) Setup(context.Context, momo.PinnedConn, SessionOptions) (func() error, error) {
	return nil, nil
}
func (renderStub) Lock(context.Context, momo.PinnedConn, string) (func() error, func(context.Context) (string, error), error) {
	return nil, nil, nil
}
func (renderStub) Render(plan []Change) ([]Statement, error) {
	var out []Statement
	for _, c := range plan {
		if c.Kind == CreateView {
			return nil, fmt.Errorf("render view %s: %w", c.Name, momo.ErrUnsupported)
		}
		out = append(out, Statement{SQL: string(c.Kind) + " " + c.Name})
	}
	return out, nil
}

// Catches: a step the dialect cannot render failing the whole repair or, worse,
// being printed as made-up SQL; the rest must still be rendered.
func TestRenderRepairListsUnsupportedSteps(t *testing.T) {
	ordered := []Change{
		{Kind: AddColumn, Table: "t", Name: "c", After: &schema.Column{Name: "c", Type: "int", Nullable: true}},
		{Kind: CreateView, Name: "v"},
	}
	changes, stmts, unsupported, err := renderRepair(renderStub{}, &schema.Schema{}, &schema.Schema{}, ordered)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(changeKinds(changes), []ChangeKind{AddColumn}) || len(stmts) != 1 || !strings.HasPrefix(stmts[0].SQL, "add_column") {
		t.Fatalf("changes %v statements %+v, want only the add_column rendered", changeKinds(changes), stmts)
	}
	if len(unsupported) != 1 || unsupported[0].Change.Kind != CreateView || !strings.Contains(unsupported[0].Reason, "cannot render") {
		t.Fatalf("unsupported = %+v, want the view", unsupported)
	}
	r := &Repair{Unsupported: unsupported}
	if r.Complete() {
		t.Fatal("a repair with an unsupported step is not complete")
	}
}

// Catches: a difference no statement can repair (an invalid index, an
// unmanaged extension) leaving Complete true and the exit code 0.
func TestUncoveredNamesWhatNoChangeRepairs(t *testing.T) {
	rep := &DriftReport{
		Changed: []ObjectDiff{
			{Object: ObjectRef{Kind: "column", Name: "t.c"}, Detail: "type differ"},
			{Object: ObjectRef{Kind: "index", Name: "t.t_i"}, Detail: invalidIndexDetail},
		},
		Missing:   []ObjectRef{{Kind: "check", Name: "t.ck"}, {Kind: "function", Name: "f(integer)"}},
		Unmanaged: []ObjectRef{{Kind: "extension", Name: "pgcrypto"}, {Kind: "table", Name: "scratch"}},
	}
	changes := []Change{
		{Kind: AlterColumn, Table: "t", Name: "c"},
		{Kind: AddCheck, Table: "t", Name: "ck"},
		{Kind: CreateFunction, Name: "f"},
		{Kind: DropTable, Table: "scratch", Name: "scratch"},
	}
	got := uncovered(rep, changes)
	if len(got) != 2 || !strings.Contains(got[0], "index t.t_i") || !strings.Contains(got[1], "extension pgcrypto") {
		t.Fatalf("uncovered = %q, want the invalid index and the extension", got)
	}
	if (&Repair{Uncovered: got}).Complete() {
		t.Fatal("a repair with uncovered differences is not complete")
	}
}

// Catches: the staged hint telling a repair user to run `migrate diff
// --enforce-not-null`, and a staged column leaving Complete true.
func TestStagedHintsAreRewordedForARepair(t *testing.T) {
	hints := []string{
		"staged: t.c was added as nullable; backfill it, then run migrate diff with --enforce-not-null to emit ALTER TABLE t ALTER COLUMN c SET NOT NULL",
		"risk: something else",
	}
	staged, other := splitStagedHints(hints)
	want := "t.c was added as nullable; backfill it, then run ALTER TABLE t ALTER COLUMN c SET NOT NULL"
	if len(staged) != 1 || staged[0] != want || strings.Contains(staged[0], "enforce-not-null") {
		t.Fatalf("staged = %q, want %q", staged, want)
	}
	if len(other) != 1 || other[0] != "risk: something else" {
		t.Fatalf("other hints = %q", other)
	}
	if (&Repair{Staged: staged}).Complete() {
		t.Fatal("a repair that staged a column is not complete")
	}
	if !(&Repair{}).Complete() {
		t.Fatal("an empty repair is complete")
	}
}
