// SPDX-License-Identifier: Apache-2.0

package sqlscan

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

// FuzzScanRaw checks scanner invariants on arbitrary SQL for every dialect:
// no panic, verbatim text never grows beyond the input, a Dollar token
// always carries digits only.
func FuzzScanRaw(f *testing.F) {
	for _, s := range []string{
		"SELECT $1, $2", "a = $1 OR b = $1", "SELECT '$1', ?", "SELECT $q$ $1 $q$",
		"/* $1 */ -- $2\n$3", `"a$1"`, "x$1 $", "$", "$99999999999999999999", "?? @a $1",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, sql string) {
		for _, d := range []dialect.Name{dialect.Postgres, dialect.SQLite, dialect.MySQL} {
			toks, err := Scan(sql, dialect.For(d))
			if err == nil {
				var text strings.Builder
				for _, tok := range toks {
					switch tok.Kind {
					case Text:
						text.WriteString(tok.Text)
					case Dollar:
						if tok.Text == "" || strings.Trim(tok.Text, "0123456789") != "" {
							t.Fatalf("%s: Dollar %q is not digits, sql %q", d, tok.Text, sql)
						}
					}
				}
				if text.Len() > len(sql) {
					t.Fatalf("%s: text %q longer than input %q", d, text.String(), sql)
				}
			}
		}
	})
}
