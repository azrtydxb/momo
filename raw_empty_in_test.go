// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

// An empty raw list makes NOT IN never true (NULL semantics), so it must be a
// build error instead of silently returning no rows.
func TestRawEmptyInIsBuildError(t *testing.T) {
	for _, d := range []dialect.Name{dialect.Postgres, dialect.SQLite} {
		for _, sql := range []string{"x IN (?)", "x NOT IN (?)"} {
			_, err := compile(dialect.For(d), func(b *Builder) {
				b.appendRaw(sql, []any{In([]string{})})
			})
			var me *Error
			if !errors.As(err, &me) || me.Kind != ErrValidation || me.Op != "build" ||
				!strings.Contains(me.Error(), "empty IN list in raw SQL") {
				t.Fatalf("%s on %s: err = %v, want a build-time validation error", sql, d, err)
			}
		}
	}
}

func TestRawNonEmptyInBinds(t *testing.T) {
	c, err := compile(dialect.For(dialect.Postgres), func(b *Builder) {
		b.appendRaw("x NOT IN (?)", []any{In([]string{"a", "b"})})
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.SQL != "x NOT IN ($1, $2)" || len(c.Args) != 2 {
		t.Fatalf("SQL = %q args = %v", c.SQL, c.Args)
	}
}
