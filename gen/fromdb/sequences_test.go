// SPDX-License-Identifier: Apache-2.0

package fromdb

import (
	"reflect"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/schemautil"
	"github.com/azrtydxb/momo/schema"
)

// seq is a fully populated bigint sequence, as an inspector reports it.
func seq(name string, start, incr, lo, hi, cache int64) schema.Sequence {
	return schema.Sequence{Name: name, AsType: "bigint", Start: schemautil.Int64(start), Increment: incr, MinValue: schemautil.Int64(lo), MaxValue: schemautil.Int64(hi), Cache: cache}
}

func sequencesSchema(seqs ...schema.Sequence) *schema.Schema {
	return &schema.Schema{
		FormatVersion: schema.FormatVersion,
		Dialect:       dialect.Postgres,
		Sequences:     seqs,
		Tables: []schema.Table{{
			Name:       "jobs",
			Columns:    []schema.Column{{Name: "id", Type: "bigint", Default: "nextval('jobs_seq'::regclass)"}},
			PrimaryKey: []string{"id"},
		}},
	}
}

// TestSequencesAreDeclaredOnTheFirstModel is the --from-db half of the issue: a
// standalone sequence that a column default names is written as a c.Sequence
// declaration, with only the options that differ from the defaults, and stays
// in the snapshot in full.
//
// Catches: the sequence missing from the models and the snapshot, so the init
// migration fails on an empty database with `relation "jobs_seq" does not exist`.
func TestSequencesAreDeclaredOnTheFirstModel(t *testing.T) {
	inspected := seq("jobs_seq", 1, 1, 1, 9223372036854775807, 1)
	res, files := generate(t, sequencesSchema(inspected), Config{})
	if res.Failed() {
		t.Fatalf("findings: %v", res.Findings)
	}
	mustContain(t, "plain sequence", files["jobs"], `c.Sequence("jobs_seq")`)
	if len(res.Snapshot.Sequences) != 1 || !reflect.DeepEqual(res.Snapshot.Sequences[0], inspected) {
		t.Fatalf("snapshot sequences = %+v, want the inspected one", res.Snapshot.Sequences)
	}
}

func TestSequenceStatement(t *testing.T) {
	cases := []struct {
		name string
		in   schema.Sequence
		want string
	}{
		{"plain", seq("s", 1, 1, 1, 9223372036854775807, 1), `c.Sequence("s")`},
		{"zero value", schema.Sequence{Name: "s"}, `c.Sequence("s")`},
		{"step and cache", seq("s", 1, 10, 1, 9223372036854775807, 20), `c.Sequence("s").Increment(10).Cache(20)`},
		{"start apart from min", seq("s", 500, 1, 1, 9223372036854775807, 1), `c.Sequence("s").Start(500)`},
		{"zero start and min", seq("s", 0, 1, 0, 9223372036854775807, 1), `c.Sequence("s").MinValue(0)`},
		{"zero start above min", seq("s", 0, 1, -5, 9223372036854775807, 1), `c.Sequence("s").MinValue(-5).Start(0)`},
		{"bounded integer", schema.Sequence{Schema: "app", Name: "s", AsType: "integer", Start: schemautil.Int64(100), Increment: 1, MinValue: schemautil.Int64(100), MaxValue: schemautil.Int64(999), Cache: 1, Cycle: true}, `c.Sequence("s").Schema("app").As("integer").MinValue(100).MaxValue(999).Cycle()`},
		{"descending", seq("s", -1, -1, -9223372036854775808, -1, 1), `c.Sequence("s").Increment(-1)`},
		{"descending max zero", seq("s", 0, -1, -9223372036854775808, 0, 1), `c.Sequence("s").Increment(-1).MaxValue(0)`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := sequenceStmt(c.in)
			if got != c.want {
				t.Fatalf("got  %s\nwant %s", got, c.want)
			}
		})
	}
}
