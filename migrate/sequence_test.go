// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"errors"
	"reflect"
	"testing"

	"github.com/azrtydxb/momo/internal/schemautil"
	"github.com/azrtydxb/momo/schema"
)

// seqSchema is a schema with one table whose id defaults to nextval of the
// standalone sequence jobs_seq.
func seqSchema(seqs ...schema.Sequence) *schema.Schema {
	return &schema.Schema{Dialect: "postgres", Sequences: seqs, Tables: []schema.Table{{
		Name:       "jobs",
		Columns:    []schema.Column{{Name: "id", Type: "bigint", Default: "nextval('jobs_seq'::regclass)"}},
		PrimaryKey: []string{"id"},
	}}}
}

// TestDiffSequenceCreateBeforeTable is the issue's failure: the sequence a
// column default names must exist before the table is created.
//
// Catches: a create_table that runs before create_sequence, which fails on an
// empty database with `relation "jobs_seq" does not exist`.
func TestDiffSequenceCreateBeforeTable(t *testing.T) {
	changes, _, err := Diff(&schema.Schema{Dialect: "postgres"}, seqSchema(schema.Sequence{Name: "jobs_seq"}), DiffOptions{})
	if err != nil {
		t.Fatal(err)
	}
	ordered, err := OrderErr(changes)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := kinds(ordered), []ChangeKind{CreateSequence, CreateTable}; !reflect.DeepEqual(got, want) {
		t.Fatalf("kinds = %v, want %v", got, want)
	}
}

// TestDiffSequenceAlterIsOneChange: a changed increment is one alter_sequence
// carrying both sides, and an unchanged declaration is no change.
//
// Catches: a drop and create of the sequence, which would reset its counter.
func TestDiffSequenceAlterIsOneChange(t *testing.T) {
	from := seqSchema(schema.Sequence{Name: "jobs_seq", AsType: "bigint", Start: schemautil.Int64(1), Increment: 1, MinValue: schemautil.Int64(1), MaxValue: schemautil.Int64(9223372036854775807), Cache: 1})
	same, _, err := Diff(from, seqSchema(schema.Sequence{Name: "jobs_seq"}), DiffOptions{})
	if err != nil || len(same) != 0 {
		t.Fatalf("a minimal declaration of the inspected sequence must be no change, got %v, %v", same, err)
	}
	changes, _, err := Diff(from, seqSchema(schema.Sequence{Name: "jobs_seq", Increment: 5}), DiffOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Kind != AlterSequence || changes[0].Name != "jobs_seq" {
		t.Fatalf("changes = %+v, want one alter_sequence", changes)
	}
	if changes[0].Before == nil || changes[0].After == nil {
		t.Fatalf("alter_sequence must carry both sides: %+v", changes[0])
	}
}

// TestDiffSequenceExplicitZeroRoundTrips: a counter that starts at 0 and a
// descending sequence with MAXVALUE -1, each as inspected, are no change
// against their declaration, and moving the start to the default is one.
//
// Catches: an explicit 0 read as unset, which would render START WITH 1 and
// then differ from the database on every diff.
func TestDiffSequenceExplicitZeroRoundTrips(t *testing.T) {
	counter := schema.Sequence{Name: "counter_seq", Start: schemautil.Int64(0), MinValue: schemautil.Int64(0)}
	inspectedCounter := schema.Sequence{Name: "counter_seq", AsType: "bigint", Start: schemautil.Int64(0), Increment: 1, MinValue: schemautil.Int64(0), MaxValue: schemautil.Int64(9223372036854775807), Cache: 1}
	down := schema.Sequence{Name: "down_seq", Increment: -1, MaxValue: schemautil.Int64(-1)}
	inspectedDown := schema.Sequence{Name: "down_seq", AsType: "bigint", Start: schemautil.Int64(-1), Increment: -1, MinValue: schemautil.Int64(-9223372036854775808), MaxValue: schemautil.Int64(-1), Cache: 1}
	changes, _, err := Diff(seqSchema(inspectedCounter, inspectedDown), seqSchema(counter, down), DiffOptions{})
	if err != nil || len(changes) != 0 {
		t.Fatalf("a declaration of the inspected sequences must be no change, got %v, %v", changes, err)
	}
	changes, _, err = Diff(seqSchema(inspectedCounter), seqSchema(schema.Sequence{Name: "counter_seq"}), DiffOptions{})
	if err != nil || len(changes) != 1 || changes[0].Kind != AlterSequence {
		t.Fatalf("a counter that loses START 0 MINVALUE 0 is one alter_sequence, got %v, %v", changes, err)
	}
	rep := compare(seqSchema(counter, down), seqSchema(inspectedCounter, inspectedDown), false, nil)
	if !rep.Clean() {
		t.Fatalf("drift must be clean: %+v", rep)
	}
}

// TestDiffSequenceDropIsDestructiveAndLast: dropping a sequence needs
// AllowDestructive and runs after the table whose default used it is gone.
//
// Catches: DROP SEQUENCE before DROP TABLE, which PostgreSQL refuses because
// the column default depends on the sequence.
func TestDiffSequenceDropIsDestructiveAndLast(t *testing.T) {
	from := seqSchema(schema.Sequence{Name: "jobs_seq"})
	to := &schema.Schema{Dialect: "postgres"}
	if _, _, err := Diff(from, to, DiffOptions{}); !errors.Is(err, ErrDestructive) {
		t.Fatalf("err = %v, want ErrDestructive", err)
	}
	changes, _, err := Diff(from, to, DiffOptions{AllowDestructive: true})
	if err != nil {
		t.Fatal(err)
	}
	ordered, err := OrderErr(changes)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := kinds(ordered), []ChangeKind{DropTable, DropSequence}; !reflect.DeepEqual(got, want) {
		t.Fatalf("kinds = %v, want %v", got, want)
	}
}

// TestSequenceOrderWithColumnDefaultSwap: a default moved to another sequence
// creates the new one first and drops the old one after the column changed.
func TestSequenceOrderWithColumnDefaultSwap(t *testing.T) {
	from := seqSchema(schema.Sequence{Name: "jobs_seq"})
	to := seqSchema(schema.Sequence{Name: "jobs_seq2"})
	to.Tables[0].Columns[0].Default = "nextval('jobs_seq2'::regclass)"
	changes, _, err := Diff(from, to, DiffOptions{AllowDestructive: true})
	if err != nil {
		t.Fatal(err)
	}
	ordered, err := OrderErr(changes)
	if err != nil {
		t.Fatal(err)
	}
	want := []ChangeKind{CreateSequence, AlterColumn, DropSequence}
	if got := kinds(ordered); !reflect.DeepEqual(got, want) {
		t.Fatalf("kinds = %v, want %v", got, want)
	}
}

// TestCompareReportsSequences: drift sees a sequence that exists on one side
// only and one whose definition differs.
//
// Catches: a flatten that leaves sequences out, so adopting a database looks
// clean while a fresh install differs.
func TestCompareReportsSequences(t *testing.T) {
	snap := seqSchema(schema.Sequence{Name: "jobs_seq"})
	live := seqSchema(schema.Sequence{Name: "jobs_seq", Increment: 3}, schema.Sequence{Name: "extra_seq"})
	rep := compare(snap, live, false, nil)
	if len(rep.Changed) != 1 || rep.Changed[0].Object.Kind != "sequence" || rep.Changed[0].Object.Name != "jobs_seq" {
		t.Fatalf("changed = %+v, want the jobs_seq definition", rep.Changed)
	}
	if len(rep.Unmanaged) != 1 || rep.Unmanaged[0].Kind != "sequence" || rep.Unmanaged[0].Name != "extra_seq" {
		t.Fatalf("unmanaged = %+v, want extra_seq", rep.Unmanaged)
	}
	if rep := compare(snap, seqSchema(schema.Sequence{Name: "jobs_seq"}), false, nil); !rep.Clean() {
		t.Fatalf("equal sequences must be clean: %+v", rep)
	}
	if rep := compare(snap, seqSchema(), false, nil); len(rep.Missing) != 1 || rep.Missing[0].Kind != "sequence" {
		t.Fatalf("missing = %+v, want the sequence", rep.Missing)
	}
}

// TestExcludeHidesSequences: migrate.exclude hides a sequence like any object.
func TestExcludeHidesSequences(t *testing.T) {
	s := seqSchema(schema.Sequence{Name: "legacy_seq"}, schema.Sequence{Name: "jobs_seq"})
	got := applyExclude(s, []string{"legacy_*"})
	if len(got.Sequences) != 1 || got.Sequences[0].Name != "jobs_seq" {
		t.Fatalf("sequences = %+v, want only jobs_seq", got.Sequences)
	}
}

// TestAutoMigrateSequencePolicy: AutoMigrate creates a sequence but neither
// alters nor drops one.
func TestAutoMigrateSequencePolicy(t *testing.T) {
	desired := seqSchema()
	cases := []struct {
		kind ChangeKind
		skip bool
	}{{CreateSequence, false}, {AlterSequence, true}, {DropSequence, true}}
	for _, c := range cases {
		got := autoSkipReason(Change{Kind: c.kind, Name: "jobs_seq"}, desired, nil)
		if (got != "") != c.skip {
			t.Errorf("%s: skip reason %q, want skipped=%v", c.kind, got, c.skip)
		}
	}
}
