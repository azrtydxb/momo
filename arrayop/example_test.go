// SPDX-License-Identifier: Apache-2.0

package arrayop_test

import (
	"errors"
	"fmt"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/arrayop"
	"github.com/azrtydxb/momo/dialect"
)

// Contains binds the values as one array parameter, however many there are.
// Other dialects have no array columns and fail the build with ErrUnsupported.
func Example() {
	tags := momo.NewColumn[[]string]("docs", "tags", 0)
	pred := arrayop.Contains(tags, "go", "sql")

	pg := momo.NewBuilder(dialect.For(dialect.Postgres))
	pred.AppendSQL(pg)
	fmt.Println(pg.String(), pg.Args())

	lite := momo.NewBuilder(dialect.For(dialect.SQLite))
	pred.AppendSQL(lite)
	fmt.Println(errors.Is(lite.Err(), momo.ErrUnsupported))
	// Output:
	// "docs"."tags" @> $1 [[go sql]]
	// true
}

// AnyOf tests a scalar column against a list with a single bind, so the
// statement text does not change with the length of the list.
func ExampleAnyOf() {
	title := momo.NewColumn[string]("docs", "title", 0)

	b := momo.NewBuilder(dialect.For(dialect.Postgres))
	arrayop.AnyOf(title, "a", "b", "c").AppendSQL(b)
	fmt.Println(b.String(), b.Args())
	// Output:
	// "docs"."title" = ANY($1) [[a b c]]
}
