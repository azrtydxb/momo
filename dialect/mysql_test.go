// SPDX-License-Identifier: Apache-2.0

package dialect

import "testing"

func TestMySQLQuoting(t *testing.T) {
	d := MySQLFlavor(FlavorMySQL)
	if got := d.QuoteIdent("order"); got != "`order`" {
		t.Fatalf("QuoteIdent(order) = %s, want `order`", got)
	}
	if got := d.QuoteIdent("a`b"); got != "`a``b`" {
		t.Fatalf("embedded backtick = %s, want `a``b`", got)
	}
	if got := string(d.AppendPlaceholder(nil, 3)); got != "?" {
		t.Fatalf("placeholder = %s, want ?", got)
	}
}

func TestMySQLFlavorCapabilities(t *testing.T) {
	my := MySQLFlavor(FlavorMySQL)
	ma := MySQLFlavor(FlavorMariaDB)
	if my.Returning || my.InsertReturning {
		t.Fatal("MySQL must not advertise RETURNING")
	}
	if ma.Returning || !ma.InsertReturning {
		t.Fatal("MariaDB: InsertReturning true, Returning (update/delete) false in 11.8")
	}
	if len(my.Vector.Metrics) != 0 {
		t.Fatalf("MySQL Community has no distance functions, got %v", my.Vector.Metrics)
	}
	if len(ma.Vector.Metrics) != 2 || len(ma.Vector.Indexes) != 1 {
		t.Fatalf("MariaDB vector caps = %+v", ma.Vector)
	}
	if !my.OnDuplicateKey || !ma.OnDuplicateKey {
		t.Fatal("both flavors must support ON DUPLICATE KEY UPDATE")
	}
	if MySQLFlavor("oracle") != nil {
		t.Fatal("unknown flavor must return nil")
	}
	if ma.Flavor != FlavorMariaDB || my.Flavor != FlavorMySQL {
		t.Fatal("flavor not recorded")
	}
}
