// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

func TestMySQLLockRendering(t *testing.T) {
	cases := []struct {
		name   string
		flavor dialect.Flavor
		share  bool
		opts   []string
		want   string
	}{
		{"update", dialect.FlavorMySQL, false, nil, " FOR UPDATE"},
		{"share on mysql", dialect.FlavorMySQL, true, nil, " FOR SHARE"},
		{"share on mariadb", dialect.FlavorMariaDB, true, nil, " LOCK IN SHARE MODE"},
		{"update nowait", dialect.FlavorMySQL, false, []string{"NOWAIT"}, " FOR UPDATE NOWAIT"},
		{"update skip locked", dialect.FlavorMySQL, false, []string{"SKIP LOCKED"}, " FOR UPDATE SKIP LOCKED"},
		{"mariadb share nowait", dialect.FlavorMariaDB, true, []string{"NOWAIT"}, " LOCK IN SHARE MODE NOWAIT"},
	}
	for _, c := range cases {
		b := NewBuilder(dialect.MySQLFlavor(c.flavor))
		renderMySQLLock(b, c.flavor, c.share, c.opts...)
		if got := b.String(); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// Catches: the lock string stored by ForUpdate and ForShare reaching MariaDB
// as FOR SHARE, which that server rejects.
func TestRenderLockFollowsDialect(t *testing.T) {
	cases := []struct {
		d    *dialect.Dialect
		lock string
		want string
	}{
		{dialect.For(dialect.Postgres), "FOR SHARE SKIP LOCKED", " FOR SHARE SKIP LOCKED"},
		{dialect.MySQLFlavor(dialect.FlavorMySQL), "FOR SHARE", " FOR SHARE"},
		{dialect.MySQLFlavor(dialect.FlavorMariaDB), "FOR SHARE SKIP LOCKED", " LOCK IN SHARE MODE SKIP LOCKED"},
		{dialect.MySQLFlavor(dialect.FlavorMariaDB), "FOR UPDATE NOWAIT", " FOR UPDATE NOWAIT"},
	}
	for _, c := range cases {
		b := NewBuilder(c.d)
		renderLock(b, c.lock)
		if got := b.String(); got != c.want {
			t.Errorf("%s %q: got %q, want %q", c.d.Name, c.lock, got, c.want)
		}
	}
}
