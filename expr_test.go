// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
)

// Catches: Order.AppendOrder dropping the direction or the NULLS placement that
// code outside the package (an ordered aggregate) relies on.
func TestOrderAppendOrder(t *testing.T) {
	title := momo.NewColumn[string]("docs", "title", 0)
	cases := []struct {
		name string
		o    momo.Order
		want string
	}{
		{"asc", title.Asc(), `"docs"."title" ASC`},
		{"desc", title.Desc(), `"docs"."title" DESC`},
		{"desc nulls last", title.Desc().NullsLast(), `"docs"."title" DESC NULLS LAST`},
		{"asc nulls first", title.Asc().NullsFirst(), `"docs"."title" ASC NULLS FIRST`},
	}
	for _, c := range cases {
		b := momo.NewBuilder(dialect.For(dialect.Postgres))
		c.o.AppendOrder(b)
		if b.Err() != nil || b.String() != c.want {
			t.Errorf("%s: got %q, %v; want %q", c.name, b.String(), b.Err(), c.want)
		}
	}

	// MySQL has no NULLS FIRST/LAST: the order gains an IS NULL sort key.
	myCases := []struct {
		name string
		o    momo.Order
		want string
	}{
		{"asc nulls last", title.Asc().NullsLast(), "(`docs`.`title` IS NULL) ASC, `docs`.`title` ASC"},
		{"desc nulls first", title.Desc().NullsFirst(), "(`docs`.`title` IS NULL) DESC, `docs`.`title` DESC"},
		{"asc nulls first", title.Asc().NullsFirst(), "(`docs`.`title` IS NULL) DESC, `docs`.`title` ASC"},
		{"desc nulls last", title.Desc().NullsLast(), "(`docs`.`title` IS NULL) ASC, `docs`.`title` DESC"},
		{"default nulls", title.Desc(), "`docs`.`title` DESC"},
	}
	for _, c := range myCases {
		my := momo.NewBuilder(dialect.For(dialect.MySQL))
		c.o.AppendOrder(my)
		if my.Err() != nil || my.String() != c.want {
			t.Errorf("mysql %s: got %q, %v; want %q", c.name, my.String(), my.Err(), c.want)
		}
	}
}
