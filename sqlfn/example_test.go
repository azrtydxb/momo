// SPDX-License-Identifier: Apache-2.0

package sqlfn_test

import (
	"errors"
	"fmt"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/sqlfn"
)

// Every function is a value expression, so it takes the comparison methods of
// a column and can be used in Where and OrderBy.
func Example() {
	title := momo.NewColumn[string]("docs", "title", 0)
	pred := sqlfn.Lower(title).Eq("go")

	b := momo.NewBuilder(dialect.For(dialect.Postgres))
	pred.AppendSQL(b)
	fmt.Println(b.String(), b.Args())
	// Output:
	// lower("docs"."title") = $1 [go]
}

// Case builds CASE WHEN ... THEN ... ELSE ... END whose branches share one Go
// type. Without a When the build fails with ErrValidation.
func ExampleCase() {
	hits := momo.NewColumn[int64]("docs", "hits", 0)
	size := sqlfn.Case[string]().
		When(hits.Gt(100), "big").
		Else("small")

	b := momo.NewBuilder(dialect.For(dialect.Postgres))
	size.AppendSQL(b)
	fmt.Println(b.String(), b.Args())
	// Output:
	// CASE WHEN "docs"."hits" > $1 THEN $2 ELSE $3 END [100 big small]
}

// Add binds the value as a parameter; arithmetic accepts numeric columns only.
func ExampleAdd() {
	hits := momo.NewColumn[int64]("docs", "hits", 0)

	b := momo.NewBuilder(dialect.For(dialect.SQLite))
	sqlfn.Add(hits, 5).AppendSQL(b)
	fmt.Println(b.String(), b.Args())
	// Output:
	// ("docs"."hits" + ?) [5]
}

// CurrentSchema is current_schema() on PostgreSQL and fails the build with
// ErrUnsupported on SQLite, which has no schemas.
func ExampleCurrentSchema() {
	pg := momo.NewBuilder(dialect.For(dialect.Postgres))
	sqlfn.CurrentSchema().AppendSQL(pg)
	fmt.Println(pg.String())

	lite := momo.NewBuilder(dialect.For(dialect.SQLite))
	sqlfn.CurrentSchema().AppendSQL(lite)
	fmt.Println(errors.Is(lite.Err(), momo.ErrUnsupported))
	// Output:
	// current_schema()
	// true
}

// Cast validates the SQL type name, so it cannot carry SQL text.
func ExampleCast() {
	n := momo.NewColumn[int64]("docs", "n", 0)

	ok := momo.NewBuilder(dialect.For(dialect.Postgres))
	sqlfn.Cast[float64](n, "numeric(10,2)").AppendSQL(ok)
	fmt.Println(ok.String())

	bad := momo.NewBuilder(dialect.For(dialect.Postgres))
	sqlfn.Cast[float64](n, "int); DROP TABLE docs; --").AppendSQL(bad)
	fmt.Println(errors.Is(bad.Err(), momo.ErrValidation))
	// Output:
	// CAST("docs"."n" AS numeric(10,2))
	// true
}
