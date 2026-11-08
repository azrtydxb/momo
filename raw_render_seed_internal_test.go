// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

// Rendering never panics for hostile SQL whatever the argument count; the
// tokenizer's own fuzz test lives in internal/sqlscan.
func TestAppendRawNeverPanicsOnHostileSQL(t *testing.T) {
	for _, s := range []string{
		"SELECT $1, $2", "a = $1 OR b = $1", "SELECT '$1', ?", "SELECT $q$ $1 $q$",
		"/* $1 */ -- $2\n$3", `"a$1"`, "x$1 $", "$", "$99999999999999999999", "?? @a $1",
	} {
		for _, d := range []dialect.Name{dialect.Postgres, dialect.SQLite, dialect.MySQL} {
			for n := 0; n < 3; n++ {
				_, _ = compile(dialect.For(d), func(b *Builder) { b.appendRaw(s, make([]any, n)) })
			}
		}
	}
}
