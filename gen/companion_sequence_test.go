// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"reflect"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/schema"
)

// TestCompanionSequenceDeclaration reads c.Sequence with every option and
// checks the sequences arrive in the built schema, sorted.
//
// Catches: a chain option dropped or misread, a negative literal refused, and
// a sequence that never reaches the snapshot.
func TestCompanionSequenceDeclaration(t *testing.T) {
	g, diags, err := Load([]string{"./companionsequence"}, Config{Dir: fixturesDir(t)})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	s, sdiags := BuildSchema(g.Models, Config{Dialects: []dialect.Name{dialect.Postgres}})
	if len(sdiags) != 0 {
		t.Fatalf("schema diagnostics: %v", sdiags)
	}
	want := []schema.Sequence{
		{Name: "countdown_seq", Increment: -1, MinValue: schema.Int64(-9223372036854775808), MaxValue: schema.Int64(-1)},
		{Name: "counter_seq", Start: schema.Int64(0), MinValue: schema.Int64(0)},
		{Name: "jobs_seq"},
		{Schema: "app", Name: "ticket_seq", AsType: "integer", Start: schema.Int64(1000), Increment: 10, MinValue: schema.Int64(1000), MaxValue: schema.Int64(99999), Cache: 20, Cycle: true},
	}
	if len(s.Sequences) != len(want) {
		t.Fatalf("sequences = %+v, want %+v", s.Sequences, want)
	}
	for i := range want {
		if !reflect.DeepEqual(s.Sequences[i], want[i]) {
			t.Errorf("sequence %d = %+v, want %+v", i, s.Sequences[i], want[i])
		}
	}
}

// TestCompanionSequenceRefusedOffPostgres: a standalone sequence is refused for
// SQLite and for MySQL (which covers MariaDB), because no migration could create
// it.
//
// Catches: a sequence silently dropped from a SQLite or MySQL schema, leaving
// the column defaults that use it failing at run time.
func TestCompanionSequenceRefusedOffPostgres(t *testing.T) {
	g, diags, err := Load([]string{"./companionsequence"}, Config{Dir: fixturesDir(t)})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	for _, d := range []dialect.Name{dialect.SQLite, dialect.MySQL} {
		_, sdiags := BuildSchema(g.Models, Config{Dialects: []dialect.Name{d}})
		text := diagStrings(sdiags)
		if !strings.Contains(text, "a standalone sequence is not supported on "+string(d)) {
			t.Errorf("%s: diagnostics lack the refusal:\n%s", d, text)
		}
	}
}

// TestCompanionSequenceRejected: a repeated option, a name constant where a
// literal is required, a zero increment and a repeated name are diagnosed.
func TestCompanionSequenceRejected(t *testing.T) {
	_, diags, err := Load([]string{"./companionsequencebad"}, Config{Dir: fixturesDir(t)})
	if err != nil {
		t.Fatal(err)
	}
	text := diagStrings(diags)
	for _, want := range []string{
		"duplicate Cache on sequence",
		"unsupported companion expression: step",
		"Increment must not be 0",
		`sequence name ".same_seq" is already declared`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("diagnostics lack %q:\n%s", want, text)
		}
	}
}
