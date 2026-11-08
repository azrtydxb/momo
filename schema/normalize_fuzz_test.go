// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

// fuzzSchema places the fuzzed text in every field Normalized rewrites, with
// two tables listed out of order.
func fuzzSchema(d dialect.Name, in string) *Schema {
	ix := Index{Name: "i", Where: in, Parts: []IndexPart{{Expr: in}}, Concurrently: true}
	return &Schema{
		Dialect: d,
		Tables: []Table{
			{Name: "b", Columns: []Column{{Name: "c", Type: in, Default: in, Generated: in}}, Checks: []Check{{Name: "k", Expr: in}}, Indexes: []Index{ix}},
			{Name: "a"},
		},
		Views:     []View{{Name: "v", Query: in, Indexes: []Index{ix}}},
		Functions: []Function{{Name: "f", Body: in}},
		Triggers:  []Trigger{{Name: "t", Table: "a", When: in}},
		Policies:  []Policy{{Name: "p", Table: "a", Using: in, Check: in}},
	}
}

func FuzzNormalize(f *testing.F) {
	for _, s := range []string{"int4", "varchar(255)", "'a'::text", "((a))", "CURRENT_TIMESTAMP", "'x''y'", "", "TIME(0000", `'it\'S  X'`} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		for _, d := range []dialect.Name{dialect.Postgres, dialect.SQLite, dialect.MySQL} {
			once := NormalizeExpr(d, in)
			if twice := NormalizeExpr(d, once); twice != once {
				t.Fatalf("NormalizeExpr not idempotent for %q: %q then %q", in, once, twice)
			}
			if once != strings.TrimSpace(once) {
				t.Fatalf("NormalizeExpr left surrounding whitespace: %q", once)
			}
			tOnce := NormalizeType(d, in)
			if NormalizeType(d, tOnce) != tOnce {
				t.Fatalf("NormalizeType not idempotent for %q", in)
			}

			s := fuzzSchema(d, in)
			before, err := json.Marshal(s)
			if err != nil {
				t.Skip() // invalid UTF-8 is replaced by Marshal, never an error; defensive only
			}
			n1 := Normalized(s)
			if after, _ := json.Marshal(s); string(after) != string(before) {
				t.Fatalf("Normalized modified its argument for %q", in)
			}
			n2 := Normalized(n1)
			j1, _ := json.Marshal(n1)
			j2, _ := json.Marshal(n2)
			if string(j1) != string(j2) {
				t.Fatalf("Normalized not idempotent for %q", in)
			}
			if !Equal(n2, n1) {
				t.Fatalf("Normalized(Normalized(s)) not Equal to Normalized(s) for %q", in)
			}
			resorted := Normalized(n1)
			resorted.Sort()
			j3, _ := json.Marshal(resorted)
			if string(j1) != string(j3) {
				t.Fatalf("Normalized result is not in Sort order for %q", in)
			}
			if n1.Tables[0].Name != "a" {
				t.Fatalf("Normalized did not sort tables for %q", in)
			}
		}
	})
}
