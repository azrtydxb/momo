// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"bytes"
	"math"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

// full is a fully populated sequence, as an inspector reports it.
func full(name, asType string, start, incr, lo, hi int64, cycle bool) Sequence {
	return Sequence{Name: name, AsType: asType, Start: Int64(start), Increment: incr, MinValue: Int64(lo), MaxValue: Int64(hi), Cache: 1, Cycle: cycle}
}

// TestNormalizeSequenceDefaults pins the PostgreSQL defaults a declaration
// leaves out, and that an explicit 0 is kept.
//
// Catches: a declaration that names only the increment comparing unequal to the
// sequence PostgreSQL reports for it, which would show as drift and as an ALTER
// on every diff, and a START 0 MINVALUE 0 that reads back as START 1.
func TestNormalizeSequenceDefaults(t *testing.T) {
	cases := []struct {
		name string
		in   Sequence
		want Sequence
	}{
		{"bare", Sequence{Name: "s"}, full("s", "bigint", 1, 1, 1, math.MaxInt64, false)},
		{"integer", Sequence{Name: "s", AsType: "int4"}, full("s", "integer", 1, 1, 1, math.MaxInt32, false)},
		{"smallint", Sequence{Name: "s", AsType: "SMALLINT"}, full("s", "smallint", 1, 1, 1, math.MaxInt16, false)},
		{"step", Sequence{Name: "s", Increment: 5, Cycle: true}, full("s", "bigint", 1, 5, 1, math.MaxInt64, true)},
		{"start follows min", Sequence{Name: "s", MinValue: Int64(100)}, full("s", "bigint", 100, 1, 100, math.MaxInt64, false)},
		{"zero start and min", Sequence{Name: "s", Start: Int64(0), MinValue: Int64(0)}, full("s", "bigint", 0, 1, 0, math.MaxInt64, false)},
		{"zero min start follows", Sequence{Name: "s", MinValue: Int64(0)}, full("s", "bigint", 0, 1, 0, math.MaxInt64, false)},
		{"descending", Sequence{Name: "s", Increment: -1}, full("s", "bigint", -1, -1, math.MinInt64, -1, false)},
		{"descending max zero", Sequence{Name: "s", Increment: -1, MaxValue: Int64(0), MinValue: Int64(-10)}, full("s", "bigint", 0, -1, -10, 0, false)},
		{"descending integer", Sequence{Name: "s", AsType: "integer", Increment: -2}, full("s", "integer", -1, -2, math.MinInt32, -1, false)},
		{"explicit", Sequence{Schema: "app", Name: "s", Start: Int64(7), Increment: 3, MinValue: Int64(5), MaxValue: Int64(90), Cache: 20}, Sequence{Schema: "app", Name: "s", AsType: "bigint", Start: Int64(7), Increment: 3, MinValue: Int64(5), MaxValue: Int64(90), Cache: 20}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := NormalizeSequence(c.in)
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("NormalizeSequence(%+v)\n got %s\nwant %s", c.in, show(got), show(c.want))
			}
			if again := NormalizeSequence(got); !reflect.DeepEqual(again, got) {
				t.Fatalf("not idempotent: %s then %s", show(got), show(again))
			}
		})
	}
}

func show(q Sequence) string {
	v := func(p *int64) string {
		if p == nil {
			return "nil"
		}
		return strconv.FormatInt(*p, 10)
	}
	return q.Name + " start=" + v(q.Start) + " min=" + v(q.MinValue) + " max=" + v(q.MaxValue) +
		" incr=" + strconv.FormatInt(q.Increment, 10) + " cache=" + strconv.FormatInt(q.Cache, 10)
}

// TestSequenceDeclarationEqualsInspection compares a minimal declaration with
// the fully populated form an inspector reads back.
//
// Catches: Equal and Normalized leaving a sequence as written, so a declared
// sequence differs from the same sequence inspected.
func TestSequenceDeclarationEqualsInspection(t *testing.T) {
	declared := &Schema{Dialect: dialect.Postgres, Tables: []Table{}, Sequences: []Sequence{{Name: "jobs_seq", Increment: 10}}}
	inspected := &Schema{Dialect: dialect.Postgres, Tables: []Table{}, Sequences: []Sequence{full("jobs_seq", "bigint", 1, 10, 1, math.MaxInt64, false)}}
	if !Equal(declared, inspected) {
		t.Fatal("a declaration and its inspected form must be equal")
	}
	inspected.Sequences[0].Increment = 5
	if Equal(declared, inspected) {
		t.Fatal("a different increment must not be equal")
	}
}

// TestSequenceZeroStartIsNotDefault: START 0 MINVALUE 0 and a descending
// sequence with MAXVALUE -1 are equal to their own inspection and different
// from the sequence that leaves the same fields unset.
//
// Catches: an explicit 0 read as "not set", which creates the sequence with
// start 1 and then drifts forever.
func TestSequenceZeroStartIsNotDefault(t *testing.T) {
	mk := func(q Sequence) *Schema {
		return &Schema{Dialect: dialect.Postgres, Tables: []Table{}, Sequences: []Sequence{q}}
	}
	declared := mk(Sequence{Name: "counter", Start: Int64(0), MinValue: Int64(0)})
	if !Equal(declared, mk(full("counter", "bigint", 0, 1, 0, math.MaxInt64, false))) {
		t.Fatal("START 0 MINVALUE 0 must equal its inspection")
	}
	if Equal(declared, mk(Sequence{Name: "counter"})) {
		t.Fatal("START 0 MINVALUE 0 must differ from the defaults")
	}
	down := mk(Sequence{Name: "down", Increment: -1, MaxValue: Int64(-1)})
	if !Equal(down, mk(full("down", "bigint", -1, -1, math.MinInt64, -1, false))) {
		t.Fatal("a descending sequence with MAXVALUE -1 must equal its inspection")
	}
}

// TestSequencesSortedAndRoundTrip checks the snapshot form: the sequences are
// written under "sequences", sorted by schema and name, an explicit 0 survives,
// and the fields read back as written.
//
// Catches: a snapshot that loses the sequences, writes them in map order or
// drops a zero.
func TestSequencesSortedAndRoundTrip(t *testing.T) {
	s := &Schema{Dialect: dialect.Postgres, Tables: []Table{}, Sequences: []Sequence{
		{Name: "z_seq", Increment: 2},
		{Schema: "app", Name: "a_seq", AsType: "integer", Cycle: true},
		{Name: "a_seq", Start: Int64(0), MinValue: Int64(0)},
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
	if z := got.Sequences[0]; z.Start == nil || *z.Start != 0 || z.MinValue == nil || *z.MinValue != 0 || z.MaxValue != nil {
		t.Fatalf("an explicit 0 must survive and an unset field stay unset: %+v", z)
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
