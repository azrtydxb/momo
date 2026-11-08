// SPDX-License-Identifier: Apache-2.0

package viz

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/internal/golden"
	"github.com/azrtydxb/momo/schema"
)

// hostile returns names with quotes, angle brackets, spaces, a carriage
// return and a line break: the characters that end or corrupt a statement in
// Mermaid or DOT.
func hostile() ([]schema.Table, []Edge) {
	tables := []schema.Table{{
		Name:       `we"ird<t>`,
		PrimaryKey: []string{"a b"},
		Columns: []schema.Column{
			{Name: "a b", Type: "int[]"},
			{Name: `q"<>&z`, Type: "text"},
		},
	}}
	edges := []Edge{{From: `we"ird<t>`, To: "p r", Cardinality: OneToMany, Label: "l\"1\r\n2"}}
	return tables, edges
}

// Catches: a name or label that breaks out of its quoting, so a table name
// from a hostile snapshot could inject Mermaid or DOT statements.
func TestHostileNamesGolden(t *testing.T) {
	tables, edges := hostile()
	for _, c := range []struct {
		golden string
		got    []byte
	}{
		{"testdata/hostile.mmd", Mermaid(tables, edges)},
		{"testdata/hostile.dot", DOT(tables, edges)},
	} {
		ok, err := golden.Diff(c.golden, c.got, *update)
		if err != nil || !ok {
			t.Errorf("%s differs (err=%v); got:\n%s", c.golden, err, c.got)
		}
	}
}

// Catches: a line break inside a Mermaid entity name, attribute or label,
// which would start a new statement.
func TestMermaidNoLineBreakInNames(t *testing.T) {
	tables := []schema.Table{{Name: "a\nb\rc", Columns: []schema.Column{{Name: "x\ny", Type: "t\nu"}}}}
	got := string(Mermaid(tables, []Edge{{From: "a\nb", To: "c", Cardinality: ManyToMany, Label: "l\nm"}}))
	if want := 2 + 3 + 1; strings.Count(got, "\n") != want {
		t.Fatalf("want %d lines, got:\n%q", want, got)
	}
}
