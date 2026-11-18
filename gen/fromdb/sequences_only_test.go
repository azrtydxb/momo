// SPDX-License-Identifier: Apache-2.0

package fromdb

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/schema"
)

// onlySequencesSchema has three tables and two standalone sequences: a_first
// (the first model, no sequence), y_outside with y_outside_seq behind the
// default of its id, and z_listed with z_listed_seq the same way.
func onlySequencesSchema() *schema.Schema {
	tbl := func(name, seqName string) schema.Table {
		def := ""
		if seqName != "" {
			def = "nextval('" + seqName + "'::regclass)"
		}
		return schema.Table{
			Name:       name,
			Columns:    []schema.Column{{Name: "id", Type: "bigint", Default: def}},
			PrimaryKey: []string{"id"},
		}
	}
	return &schema.Schema{
		FormatVersion: schema.FormatVersion,
		Dialect:       dialect.Postgres,
		Sequences: []schema.Sequence{
			seq("y_outside_seq", 1, 1, 1, 9223372036854775807, 1),
			seq("z_listed_seq", 1, 1, 1, 9223372036854775807, 1),
		},
		Tables: []schema.Table{tbl("a_first", ""), tbl("y_outside", "y_outside_seq"), tbl("z_listed", "z_listed_seq")},
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

// onlyListed generates onlySequencesSchema after edit, with only listing a_first
// and z_listed, and returns the generated files and the snapshot sequence names.
func onlyListed(t *testing.T, edit func(s *schema.Schema)) (map[string]string, string, *Result) {
	t.Helper()
	s := onlySequencesSchema()
	edit(s)
	res, files := generate(t, s, Config{Only: []string{"a_first", "z_listed"}})
	if res.Failed() {
		t.Fatalf("findings: %v", res.Findings)
	}
	return files, snapshotSequenceNames(res), res
}

func tableNamed(s *schema.Schema, name string) *schema.Table {
	for i := range s.Tables {
		if s.Tables[i].Name == name {
			return &s.Tables[i]
		}
	}
	panic("no table " + name)
}

// TestBareSeqName pins how a nextval literal is reduced to a sequence name.
//
// Catches: a dot inside a quoted name being taken for the schema separator, and
// a doubled quote staying doubled, so the sequence is judged unused and dropped.
func TestBareSeqName(t *testing.T) {
	for _, c := range [][2]string{
		{`jobs_seq`, "jobs_seq"},
		{`app.jobs_seq`, "jobs_seq"},
		{`"app"."Jobs_Seq"`, "Jobs_Seq"},
		{`"my.seq"`, "my.seq"},
		{`public."my.seq"`, "my.seq"},
		{`"my""seq"`, `my"seq`},
		{`"a.b"."c""d.e"`, `c"d.e`},
	} {
		if got := bareSeqName(c[0]); got != c[1] {
			t.Errorf("bareSeqName(%q) = %q, want %q", c[0], got, c[1])
		}
	}
}

// TestOnlyKeepsSequencesByHowATableReachesThem covers the ways a listed table
// can use a sequence, each of which must keep it on that table's model.
//
// Catches: a use outside a plain column default (a quoted or schema qualified
// literal, a generated column, a check, an index predicate, an argument that is
// not a literal) being missed, so the sequence is dropped and the baseline
// refuses the table.
func TestOnlyKeepsSequencesByHowATableReachesThem(t *testing.T) {
	cases := []struct {
		name string
		edit func(s *schema.Schema)
	}{
		{"schema qualified literal", func(s *schema.Schema) {
			tableNamed(s, "z_listed").Columns[0].Default = "nextval('app.y_outside_seq'::regclass)"
		}},
		{"quoted literal", func(s *schema.Schema) {
			tableNamed(s, "z_listed").Columns[0].Default = `nextval('"y_outside_seq"'::regclass)`
		}},
		{"generated column", func(s *schema.Schema) {
			tableNamed(s, "z_listed").Columns[0].Default = ""
			tableNamed(s, "z_listed").Columns = append(tableNamed(s, "z_listed").Columns, schema.Column{Name: "n", Type: "bigint", Generated: "nextval('y_outside_seq'::regclass)"})
		}},
		{"check", func(s *schema.Schema) {
			tableNamed(s, "z_listed").Checks = []schema.Check{{Name: "c", Expr: "id < nextval('y_outside_seq'::regclass)"}}
		}},
		{"index predicate", func(s *schema.Schema) {
			tableNamed(s, "z_listed").Indexes = []schema.Index{{Name: "i", Parts: []schema.IndexPart{{Column: "id"}}, Where: "id < nextval('y_outside_seq'::regclass)"}}
		}},
		{"argument that is not a literal", func(s *schema.Schema) {
			tableNamed(s, "z_listed").Columns[0].Default = "nextval(('y_' || 'outside_seq')::regclass)"
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			files, _, _ := onlyListed(t, c.edit)
			// The edited table also loses its own sequence when it no longer
			// names it, so only the sequence the case points at is asserted.
			mustContain(t, "z_listed", files["z_listed"], `c.Sequence("y_outside_seq")`)
			if strings.Contains(files["a_first"], "y_outside_seq") {
				t.Errorf("a_first declares y_outside_seq, which z_listed uses:\n%s", files["a_first"])
			}
		})
	}
}

// TestOnlyHostsASharedSequenceOnTheFirstUser: two listed tables use one
// sequence, and it is declared once, on the first of them.
func TestOnlyHostsASharedSequenceOnTheFirstUser(t *testing.T) {
	files, names, _ := onlyListed(t, func(s *schema.Schema) {
		tableNamed(s, "a_first").Columns[0].Default = "nextval('y_outside_seq'::regclass)"
		tableNamed(s, "z_listed").Columns[0].Default = "nextval('y_outside_seq'::regclass)"
	})
	mustContain(t, "a_first", files["a_first"], `c.Sequence("y_outside_seq")`)
	if strings.Contains(files["z_listed"], "c.Sequence(") {
		t.Errorf("z_listed declares a sequence the first user already declares:\n%s", files["z_listed"])
	}
	if names != "y_outside_seq" {
		t.Fatalf("snapshot sequences = %q, want y_outside_seq once", names)
	}
}

// TestOnlyKeepsASequenceAFunctionOrViewUses covers the uses that are not in the
// table at all: the body of a function a listed table calls, and a view.
//
// Catches: a sequence that a kept function or view reads being dropped because
// only column defaults and checks were scanned.
func TestOnlyKeepsASequenceAFunctionOrViewUses(t *testing.T) {
	fn := func(name, body string) schema.Function {
		return schema.Function{Name: name, Args: "", Returns: "bigint", Language: "sql", Volatility: "VOLATILE", Body: body}
	}
	t.Run("function a listed table calls", func(t *testing.T) {
		files, names, _ := onlyListed(t, func(s *schema.Schema) {
			s.Functions = []schema.Function{fn("next_code", "SELECT nextval('y_outside_seq'::regclass)")}
			tableNamed(s, "z_listed").Columns[0].Default = "next_code()"
		})
		mustContain(t, "z_listed", files["z_listed"], `c.Sequence("y_outside_seq")`)
		if strings.Contains(files["a_first"], "y_outside_seq") || !strings.Contains(names, "y_outside_seq") {
			t.Fatalf("a_first:\n%s\nsnapshot sequences = %q", files["a_first"], names)
		}
	})
	t.Run("function only an unlisted table calls", func(t *testing.T) {
		files, names, _ := onlyListed(t, func(s *schema.Schema) {
			s.Functions = []schema.Function{fn("next_code", "SELECT nextval('y_outside_seq'::regclass)")}
			tableNamed(s, "y_outside").Columns[0].Default = "next_code()"
		})
		for name, src := range files {
			if strings.Contains(src, "y_outside_seq") {
				t.Errorf("%s declares y_outside_seq, which only an unlisted table reaches:\n%s", name, src)
			}
		}
		if strings.Contains(names, "y_outside_seq") {
			t.Fatalf("snapshot sequences = %q, want no y_outside_seq", names)
		}
	})
	t.Run("view", func(t *testing.T) {
		files, _, _ := onlyListed(t, func(s *schema.Schema) {
			s.Views = []schema.View{{Name: "v", Query: "SELECT nextval('y_outside_seq'::regclass) AS n"}}
		})
		mustContain(t, "a_first", files["a_first"], `c.Sequence("y_outside_seq")`)
	})
}

// TestOnlyNotesTheExcludeForAnOmittedSequence: a sequence left out still exists
// in the database, so drift and baseline see it as unmanaged; the finding says
// to add it to migrate.exclude.
func TestOnlyNotesTheExcludeForAnOmittedSequence(t *testing.T) {
	_, _, res := onlyListed(t, func(s *schema.Schema) {})
	for _, f := range res.Findings {
		if f.Object == "y_outside_seq" {
			if !strings.Contains(f.Msg, "migrate.exclude") {
				t.Fatalf("finding %q does not point at migrate.exclude", f.Msg)
			}
			return
		}
	}
	t.Fatalf("no finding for the omitted sequence y_outside_seq: %v", res.Findings)
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
