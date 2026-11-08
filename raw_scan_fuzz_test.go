// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

// FuzzScanRaw checks scanner invariants on arbitrary SQL for every dialect:
// no panic, verbatim text never grows beyond the input, a rawDollar token
// always carries digits only, and rendering never panics whatever the
// argument count.
func FuzzScanRaw(f *testing.F) {
	for _, s := range []string{
		"SELECT $1, $2", "a = $1 OR b = $1", "SELECT '$1', ?", "SELECT $q$ $1 $q$",
		"/* $1 */ -- $2\n$3", `"a$1"`, "x$1 $", "$", "$99999999999999999999", "?? @a $1",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, sql string) {
		for _, d := range []dialect.Name{dialect.Postgres, dialect.SQLite, dialect.MySQL} {
			toks, err := scanRaw(sql, dialect.For(d))
			if err == nil {
				var text strings.Builder
				for _, tok := range toks {
					switch tok.kind {
					case rawText:
						text.WriteString(tok.text)
					case rawDollar:
						if tok.text == "" || strings.Trim(tok.text, "0123456789") != "" {
							t.Fatalf("%s: rawDollar %q is not digits, sql %q", d, tok.text, sql)
						}
					}
				}
				if text.Len() > len(sql) {
					t.Fatalf("%s: text %q longer than input %q", d, text.String(), sql)
				}
			}
			for n := 0; n < 3; n++ {
				_, _ = compile(dialect.For(d), func(b *Builder) { b.appendRaw(sql, make([]any, n)) })
			}
		}
	})
}
