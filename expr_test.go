// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
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

	// MySQL has no NULLS FIRST/LAST: the build fails instead of emitting it.
	my := momo.NewBuilder(dialect.For(dialect.MySQL))
	title.Asc().NullsLast().AppendOrder(my)
	if !errors.Is(my.Err(), momo.ErrUnsupported) {
		t.Errorf("mysql nulls last: err = %v; want ErrUnsupported", my.Err())
	}
}
