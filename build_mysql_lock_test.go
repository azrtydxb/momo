// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"errors"
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

func TestMySQLLockRendering(t *testing.T) {
	cases := []struct {
		name   string
		flavor dialect.Flavor
		share  bool
		of     []string
		opts   []string
		want   string
	}{
		{"update", dialect.FlavorMySQL, false, nil, nil, " FOR UPDATE"},
		{"share on mysql", dialect.FlavorMySQL, true, nil, nil, " FOR SHARE"},
		{"share on mariadb", dialect.FlavorMariaDB, true, nil, nil, " LOCK IN SHARE MODE"},
		{"update nowait", dialect.FlavorMySQL, false, nil, []string{"NOWAIT"}, " FOR UPDATE NOWAIT"},
		{"update skip locked", dialect.FlavorMySQL, false, nil, []string{"SKIP LOCKED"}, " FOR UPDATE SKIP LOCKED"},
		{"mariadb share nowait", dialect.FlavorMariaDB, true, nil, []string{"NOWAIT"}, " LOCK IN SHARE MODE NOWAIT"},
		{"mysql of", dialect.FlavorMySQL, false, []string{"a", "b"}, []string{"SKIP LOCKED"}, " FOR UPDATE OF `a`, `b` SKIP LOCKED"},
	}
	for _, c := range cases {
		b := NewBuilder(dialect.MySQLFlavor(c.flavor))
		renderMySQLLock(b, c.flavor, c.share, c.of, c.opts...)
		if got := b.String(); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// Catches: MariaDB silently dropping or mis-rendering FOR UPDATE OF, which it
// does not support.
func TestMariaDBLockOfUnsupported(t *testing.T) {
	b := NewBuilder(dialect.MySQLFlavor(dialect.FlavorMariaDB))
	renderMySQLLock(b, dialect.FlavorMariaDB, false, []string{"a"})
	if err := b.Err(); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("err = %v, want ErrUnsupported", err)
	}
}

// Catches: a bare OFFSET reaching MySQL or MariaDB, where it is a syntax error.
func TestMySQLBareOffsetGetsLimit(t *testing.T) {
	s := selectCore{limit: -1, offset: 5}
	b := NewBuilder(dialect.MySQLFlavor(dialect.FlavorMySQL))
	s.writeLimitOffset(b)
	if got, want := b.String(), " LIMIT 18446744073709551615 OFFSET ?"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	pg := NewBuilder(dialect.For(dialect.Postgres))
	s.writeLimitOffset(pg)
	if got, want := pg.String(), " OFFSET $1"; got != want {
		t.Fatalf("postgres got %q, want %q", got, want)
	}
}
