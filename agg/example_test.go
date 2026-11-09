// SPDX-License-Identifier: Apache-2.0

package agg_test

import (
	"errors"
	"fmt"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/agg"
	"github.com/azrtydxb/momo/dialect"
)

// String keeps its ORDER BY inside the call, and the Call compares like any
// other expression, so it composes with the core aggregate momo.CountAll in
// one HAVING predicate.
func Example() {
	title := momo.NewColumn[string]("docs", "title", 0)
	having := momo.And(
		momo.CountAll().Gt(1),
		agg.String(title, ", ").OrderBy(title.Asc()).Ne("none"),
	)

	b := momo.NewBuilder(dialect.For(dialect.Postgres))
	having.AppendSQL(b)
	fmt.Println(b.String(), b.Args())
	// Output:
	// (COUNT(*) > $1 AND string_agg("docs"."title", ', ' ORDER BY "docs"."title" ASC) <> $2) [1 none]
}

// PercentileCont is an ordered-set aggregate: the ordering comes from
// WithinGroup, and only PostgreSQL has it. Other dialects fail the build with
// ErrUnsupported.
func ExamplePercentileCont() {
	score := momo.NewColumn[float64]("runs", "score", 0)
	median := agg.PercentileCont(0.5).WithinGroup(score.Asc())

	pg := momo.NewBuilder(dialect.For(dialect.Postgres))
	median.AppendSQL(pg)
	fmt.Println(pg.String())

	lite := momo.NewBuilder(dialect.For(dialect.SQLite))
	median.AppendSQL(lite)
	fmt.Println(errors.Is(lite.Err(), momo.ErrUnsupported))
	// Output:
	// percentile_cont(0.5) WITHIN GROUP (ORDER BY "runs"."score" ASC)
	// true
}

// BuildObject and JSONB build a JSON array of objects; EmptyJSONArray is the
// fallback when the group has no rows.
func ExampleJSONB() {
	title := momo.NewColumn[string]("docs", "title", 0)
	type item struct {
		Label string `json:"label"`
	}
	obj := agg.BuildObject[item](agg.Field("label", title))
	list := agg.JSONB[item](obj)

	b := momo.NewBuilder(dialect.For(dialect.Postgres))
	list.AppendSQL(b)
	fmt.Println(b.String())

	e := momo.NewBuilder(dialect.For(dialect.Postgres))
	agg.EmptyJSONArray[item]().AppendSQL(e)
	fmt.Println(e.String())
	// Output:
	// jsonb_agg(jsonb_build_object('label', "docs"."title"))
	// '[]'::jsonb
}
