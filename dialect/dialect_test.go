// SPDX-License-Identifier: Apache-2.0

package dialect

import "testing"

func TestDialectCapabilities(t *testing.T) {
	pg := For(Postgres)
	if pg == nil || pg.MaxParams != 65535 || !pg.Returning || !pg.Locking || !pg.Merge || !pg.WindowFunctions {
		t.Fatalf("postgres capabilities wrong: %+v", pg)
	}
	sq := For(SQLite)
	if sq == nil || sq.MaxParams != 32766 || !sq.Returning || sq.Locking || sq.ILike || sq.Merge {
		t.Fatalf("sqlite capabilities wrong: %+v", sq)
	}
	if For("oracle") != nil {
		t.Fatal("For must return nil for unknown names")
	}
	my := MySQLFlavor(FlavorMySQL)
	if my.Name != MySQL || my.Flavor != FlavorMySQL || my.MaxParams != 65535 {
		t.Fatalf("mysql flavor wrong: %+v", my)
	}
	if !my.OnDuplicateKey || my.OnConflict || my.NullsOrdering || my.ILike {
		t.Fatalf("mysql capability booleans wrong: %+v", my)
	}
	if MySQLFlavor(FlavorMariaDB).Flavor != FlavorMariaDB {
		t.Fatal("MariaDB flavour not recorded")
	}
}

func TestAppendIdentQuoting(t *testing.T) {
	cases := []struct {
		d    *Dialect
		in   string
		want string
	}{
		{For(Postgres), `order`, `"order"`},
		{For(Postgres), `a"b`, `"a""b"`},
		{For(SQLite), `select`, `"select"`},
		{MySQLFlavor(FlavorMySQL), `order`, "`order`"},
		{MySQLFlavor(FlavorMySQL), "a`b", "`a``b`"},
		{MySQLFlavor(FlavorMariaDB), "a`b", "`a``b`"},
	}
	for _, c := range cases {
		if got := c.d.QuoteIdent(c.in); got != c.want {
			t.Errorf("%s QuoteIdent(%q) = %s, want %s", c.d.Name, c.in, got, c.want)
		}
	}
}

func TestPlaceholderStyle(t *testing.T) {
	if got := string(For(Postgres).AppendPlaceholder(nil, 3)); got != "$3" {
		t.Errorf("postgres placeholder = %q, want $3", got)
	}
	for _, d := range []*Dialect{For(SQLite), MySQLFlavor(FlavorMySQL)} {
		if got := string(d.AppendPlaceholder(nil, 3)); got != "?" {
			t.Errorf("%s placeholder = %q, want ?", d.Name, got)
		}
	}
}
