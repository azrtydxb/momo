// SPDX-License-Identifier: Apache-2.0

package fts_test

import (
	"errors"
	"fmt"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/fts"
	"github.com/azrtydxb/momo/types"
)

// Match renders vec @@ query. The configuration and the query text are bound
// parameters, never SQL. Other dialects have no full-text search and fail the
// build with ErrUnsupported.
func Example() {
	body := momo.NewColumn[types.TSVector]("articles", "search_tsv", 0)
	pred := fts.Match(body, fts.WebsearchToQuery("english", "red fox"))

	pg := momo.NewBuilder(dialect.For(dialect.Postgres))
	pred.AppendSQL(pg)
	fmt.Println(pg.String(), pg.Args())

	lite := momo.NewBuilder(dialect.For(dialect.SQLite))
	pred.AppendSQL(lite)
	fmt.Println(errors.Is(lite.Err(), momo.ErrUnsupported))
	// Output:
	// "articles"."search_tsv" @@ websearch_to_tsquery(CAST($1 AS regconfig), $2) [english red fox]
	// true
}

// RankCD scores a match so a query can order best first. The result is an
// expression, so it also has As, Asc, Desc and the comparison methods.
func ExampleRankCD() {
	body := momo.NewColumn[types.TSVector]("articles", "search_tsv", 0)
	rank := fts.RankCD(body, fts.WebsearchToQuery("english", "fox"), 1)

	b := momo.NewBuilder(dialect.For(dialect.Postgres))
	rank.Desc().Expr.AppendSQL(b)
	fmt.Println(b.String(), b.Args())
	// Output:
	// ts_rank_cd("articles"."search_tsv", websearch_to_tsquery(CAST($1 AS regconfig), $2), 1) [english fox]
}

// Headline marks the query matches up in the original text. The options
// string is bound as a parameter.
func ExampleHeadline() {
	text := momo.NewColumn[string]("articles", "body", 0)
	q := fts.PlainToQuery("english", "fox")

	b := momo.NewBuilder(dialect.For(dialect.Postgres))
	fts.Headline("english", text, q, "StartSel=<b>, StopSel=</b>").AppendSQL(b)
	fmt.Println(b.String(), b.Args())
	// Output:
	// ts_headline(CAST($1 AS regconfig), "articles"."body", plainto_tsquery(CAST($2 AS regconfig), $3), $4) [english english fox StartSel=<b>, StopSel=</b>]
}
