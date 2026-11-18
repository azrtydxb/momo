// SPDX-License-Identifier: Apache-2.0

package fromdb

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/schema"
)

// onlySequencesSchema has three tables, each with a standalone sequence behind
// the default of its id: a_first, z_listed and y_outside.
func onlySequencesSchema() *schema.Schema {
	tbl := func(name, seqName string) schema.Table {
		return schema.Table{
			Name:       name,
			Columns:    []schema.Column{{Name: "id", Type: "bigint", Default: "nextval('" + seqName + "'::regclass)"}},
			PrimaryKey: []string{"id"},
		}
	}
	return &schema.Schema{
		FormatVersion: schema.FormatVersion,
		Dialect:       dialect.Postgres,
		Sequences: []schema.Sequence{
			seq("a_first_seq", 1, 1, 1, 9223372036854775807, 1),
			seq("y_outside_seq", 1, 1, 1, 9223372036854775807, 1),
			seq("z_listed_seq", 1, 1, 1, 9223372036854775807, 1),
		},
		Tables: []schema.Table{tbl("a_first", "a_first_seq"), tbl("y_outside", "y_outside_seq"), tbl("z_listed", "z_listed_seq")},
	}
}

func snapshotSequenceNames(res *Result) string {
	var names []string
	for _, q := range res.Snapshot.Sequences {
		names = append(names, q.Name)
	}
	return strings.Join(names, ",")
}

// TestOnlyDeclaresASequenceOnTheTableThatUsesIt is issue 231: with
// introspect.only set, a standalone sequence that only a table outside the list
// uses is neither declared nor kept in the snapshot, and one that a listed table
// uses is declared on that table's model, not on the first model.
//
// Catches: every sequence of the schema landing on the first generated model, so
// a sequence of an unlisted table is declared and the init migration creates it.
func TestOnlyDeclaresASequenceOnTheTableThatUsesIt(t *testing.T) {
	res, files := generate(t, onlySequencesSchema(), Config{Only: []string{"a_first", "z_listed"}})
	if res.Failed() {
		t.Fatalf("findings: %v", res.Findings)
	}
	if strings.Contains(files["a_first"], "c.Sequence(") {
		t.Errorf("a_first declares a sequence it does not use:\n%s", files["a_first"])
	}
	mustContain(t, "z_listed", files["z_listed"], `c.Sequence("z_listed_seq")`)
	for name, src := range files {
		if strings.Contains(src, "y_outside_seq") {
			t.Errorf("%s declares the sequence of the unlisted table y_outside:\n%s", name, src)
		}
	}
	if got := snapshotSequenceNames(res); got != "z_listed_seq" {
		t.Fatalf("snapshot sequences = %q, want only z_listed_seq", got)
	}
}

// TestOnlyWithNoListedUserDeclaresNoSequence is the first acceptance case: only
// lists one table and the sequence belongs to another.
func TestOnlyWithNoListedUserDeclaresNoSequence(t *testing.T) {
	res, files := generate(t, onlySequencesSchema(), Config{Only: []string{"a_first"}})
	if res.Failed() {
		t.Fatalf("findings: %v", res.Findings)
	}
	for name, src := range files {
		if strings.Contains(src, "c.Sequence(") {
			t.Errorf("%s declares a sequence although no listed table uses one:\n%s", name, src)
		}
	}
	if len(res.Snapshot.Sequences) != 0 {
		t.Fatalf("snapshot sequences = %v, want none", snapshotSequenceNames(res))
	}
}

// TestOnlyKeepsASequenceItNames covers a sequence that no table uses but that
// only lists by its own name: it goes on the first model.
func TestOnlyKeepsASequenceItNames(t *testing.T) {
	res, files := generate(t, onlySequencesSchema(), Config{Only: []string{"a_first", "y_outside_seq"}})
	if res.Failed() {
		t.Fatalf("findings: %v", res.Findings)
	}
	mustContain(t, "a_first", files["a_first"], `c.Sequence("y_outside_seq")`)
	if strings.Contains(files["a_first"], "z_listed_seq") {
		t.Errorf("a_first declares a sequence nothing listed uses:\n%s", files["a_first"])
	}
}
