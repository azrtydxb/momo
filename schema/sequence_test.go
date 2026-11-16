// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"bytes"
	"math"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

// TestNormalizeSequenceDefaults pins the PostgreSQL defaults a declaration
// leaves out.
//
// Catches: a declaration that names only the increment comparing unequal to the
// sequence PostgreSQL reports for it, which would show as drift and as an ALTER
// on every diff.
func TestNormalizeSequenceDefaults(t *testing.T) {
	cases := []struct {
		name string
		in   Sequence
		want Sequence
	}{
		{"bare", Sequence{Name: "s"}, Sequence{Name: "s", AsType: "bigint", Start: 1, Increment: 1, MinValue: 1, MaxValue: math.MaxInt64, Cache: 1}},
		{"integer", Sequence{Name: "s", AsType: "int4"}, Sequence{Name: "s", AsType: "integer", Start: 1, Increment: 1, MinValue: 1, MaxValue: math.MaxInt32, Cache: 1}},
		{"smallint", Sequence{Name: "s", AsType: "SMALLINT"}, Sequence{Name: "s", AsType: "smallint", Start: 1, Increment: 1, MinValue: 1, MaxValue: math.MaxInt16, Cache: 1}},
		{"step", Sequence{Name: "s", Increment: 5, Cycle: true}, Sequence{Name: "s", AsType: "bigint", Start: 1, Increment: 5, MinValue: 1, MaxValue: math.MaxInt64, Cache: 1, Cycle: true}},
		{"start follows min", Sequence{Name: "s", MinValue: 100}, Sequence{Name: "s", AsType: "bigint", Start: 100, Increment: 1, MinValue: 100, MaxValue: math.MaxInt64, Cache: 1}},
		{"descending", Sequence{Name: "s", Increment: -1}, Sequence{Name: "s", AsType: "bigint", Start: -1, Increment: -1, MinValue: math.MinInt64, MaxValue: -1, Cache: 1}},
		{"descending integer", Sequence{Name: "s", AsType: "integer", Increment: -2}, Sequence{Name: "s", AsType: "integer", Start: -1, Increment: -2, MinValue: math.MinInt32, MaxValue: -1, Cache: 1}},
		{"explicit", Sequence{Schema: "app", Name: "s", Start: 7, Increment: 3, MinValue: 5, MaxValue: 90, Cache: 20}, Sequence{Schema: "app", Name: "s", AsType: "bigint", Start: 7, Increment: 3, MinValue: 5, MaxValue: 90, Cache: 20}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := NormalizeSequence(c.in)
			if got != c.want {
				t.Fatalf("NormalizeSequence(%+v)\n got %+v\nwant %+v", c.in, got, c.want)
			}
			if again := NormalizeSequence(got); again != got {
				t.Fatalf("not idempotent: %+v then %+v", got, again)
			}
		})
	}
}

// TestSequenceDeclarationEqualsInspection compares a minimal declaration with
// the fully populated form an inspector reads back.
//
// Catches: Equal and Normalized leaving a sequence as written, so a declared
// sequence differs from the same sequence inspected.
func TestSequenceDeclarationEqualsInspection(t *testing.T) {
	declared := &Schema{Dialect: dialect.Postgres, Tables: []Table{}, Sequences: []Sequence{{Name: "jobs_seq", Increment: 10}}}
	inspected := &Schema{Dialect: dialect.Postgres, Tables: []Table{}, Sequences: []Sequence{
		{Name: "jobs_seq", AsType: "bigint", Start: 1, Increment: 10, MinValue: 1, MaxValue: math.MaxInt64, Cache: 1},
	}}
	if !Equal(declared, inspected) {
		t.Fatal("a declaration and its inspected form must be equal")
	}
	inspected.Sequences[0].Increment = 5
	if Equal(declared, inspected) {
		t.Fatal("a different increment must not be equal")
	}
}

// TestSequencesSortedAndRoundTrip checks the snapshot form: the sequences are
// written under "sequences", sorted by schema and name, and read back as
// written.
//
// Catches: a snapshot that loses the sequences or writes them in map order.
func TestSequencesSortedAndRoundTrip(t *testing.T) {
	s := &Schema{Dialect: dialect.Postgres, Tables: []Table{}, Sequences: []Sequence{
		{Name: "z_seq", Increment: 2},
		{Schema: "app", Name: "a_seq", AsType: "integer", Cycle: true},
		{Name: "a_seq"},
	}}
	var buf bytes.Buffer
	if err := s.Write(&buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"sequences": [`) {
		t.Fatalf("snapshot has no sequences key:\n%s", buf.String())
	}
	got, err := Load(&buf)
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, q := range got.Sequences {
		order = append(order, q.Schema+"."+q.Name)
	}
	if want := ".a_seq,.z_seq,app.a_seq"; strings.Join(order, ",") != want {
		t.Fatalf("order %v, want %s", order, want)
	}
	if got.Sequences[1].Increment != 2 || !got.Sequences[2].Cycle || got.Sequences[2].AsType != "integer" {
		t.Fatalf("fields lost: %+v", got.Sequences)
	}
}

// TestSnapshotWithoutSequencesLoads is the compatibility half: a snapshot
// written before sequences existed has no key and loads with none. The format
// version stays 1, so an older momo reads a newer snapshot and skips the
// sequences key without error.
//
// Catches: a required sequences key, or a format version bump that locks older
// snapshots out.
func TestSnapshotWithoutSequencesLoads(t *testing.T) {
	got, err := Load(strings.NewReader(`{"format_version":1,"dialect":"postgres","tables":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Sequences) != 0 {
		t.Fatalf("sequences %v, want none", got.Sequences)
	}
	if FormatVersion != 1 {
		t.Fatalf("FormatVersion = %d: adding the optional sequences key must not bump it", FormatVersion)
	}
}
