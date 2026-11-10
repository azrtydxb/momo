// SPDX-License-Identifier: Apache-2.0

package otelmomo

import (
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

func TestSystemName(t *testing.T) {
	cases := []struct {
		d    *dialect.Dialect
		want string
	}{
		{dialect.For(dialect.Postgres), "postgresql"},
		{dialect.For(dialect.SQLite), "sqlite"},
		{dialect.MySQLFlavor(dialect.FlavorMySQL), "mysql"},
		{dialect.MySQLFlavor(dialect.FlavorMariaDB), "mariadb"},
		{nil, "other_sql"},
	}
	for _, c := range cases {
		if got := string(systemName(c.d)); got != c.want {
			t.Errorf("systemName = %q, want %q", got, c.want)
		}
	}
}
