// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

// FuzzRawArgs asserts that no argument value, whatever its bytes, ever reaches
// the SQL text: the rendered statement is byte-for-byte the template with
// dialect placeholders, and every value travels in Args. It would fail if a
// value were interpolated or a hostile value changed placeholder numbering.
func FuzzRawArgs(f *testing.F) {
	f.Add("a")
	f.Add(`x"y`)
	f.Add("'; DROP TABLE t; --")
	f.Add("? @n $1 -- /*")
	f.Fuzz(func(t *testing.T, s string) {
		want := map[dialect.Name]string{
			dialect.Postgres: "SELECT $1, $2 FROM t WHERE x IN ($3)",
			dialect.SQLite:   "SELECT ?, ? FROM t WHERE x IN (?)",
		}
		for d, sql := range want {
			c, err := compile(dialect.For(d), func(b *Builder) {
				b.appendRaw("SELECT ?, @n FROM t WHERE x IN (?)", []any{s, NamedArg{Name: "n", Value: s}, In([]string{s})})
			})
			if err != nil {
				t.Fatal(err)
			}
			if c.SQL != sql {
				t.Fatalf("%s: SQL = %q, want %q", d, c.SQL, sql)
			}
			if len(c.Args) != 3 {
				t.Fatalf("%s: %d args, want 3", d, len(c.Args))
			}
			for i, a := range c.Args {
				if a != any(s) {
					t.Fatalf("%s: arg %d = %v, want %q", d, i, a, s)
				}
			}
		}
	})
}
