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

// Catches: For handing out a pointer to the built-in description, so one
// caller that edits a capability flag or a vector slice changes the dialect
// for every other caller.
func TestForReturnsACopy(t *testing.T) {
	for _, n := range []Name{Postgres, SQLite, MySQL} {
		a, b := For(n), For(n)
		if a == b {
			t.Fatalf("For(%s) returned the same pointer twice", n)
		}
		a.Returning = !a.Returning
		a.MaxParams = -1
		if b.MaxParams == -1 || For(n).MaxParams == -1 {
			t.Fatalf("For(%s): a change to one copy reached another", n)
		}
	}
	a, b := For(Postgres), For(Postgres)
	a.Vector.Metrics[0] = "changed"
	a.Vector.Indexes[0] = "changed"
	if b.Vector.Metrics[0] != MetricL2 || b.Vector.Indexes[0] != "hnsw" {
		t.Fatalf("a change to the vector slices of one copy reached another: %+v", b.Vector)
	}
	if got := For(Postgres).Vector; got.Metrics[0] != MetricL2 || got.Indexes[0] != "hnsw" {
		t.Fatalf("a change to a copy reached the built-in dialect: %+v", got)
	}
	s := For(SQLite)
	s.Vector.Metrics[0] = "changed"
	if For(SQLite).Vector.Metrics[0] != MetricL2 {
		t.Fatal("a change to a SQLite copy reached the built-in dialect")
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
