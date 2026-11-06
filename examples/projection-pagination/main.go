// SPDX-License-Identifier: Apache-2.0

// Command projection-pagination shows how to read only the columns a screen
// needs, page through a large table with a keyset cursor, and count.
//
// Run it against a database that has the migrations applied:
//
//	DATABASE_URL=postgres://... go run ./examples/projection-pagination
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/pgxdriver"
	"github.com/azrtydxb/momo/examples/projection-pagination/models"
	"github.com/azrtydxb/momo/examples/projection-pagination/models/article"
)

func main() {
	if err := start(context.Background()); err != nil {
		log.Fatal(err)
	}
}

func start(ctx context.Context) error {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return errors.New("DATABASE_URL is not set")
	}
	pool, err := pgxdriver.NewPool(ctx, url)
	if err != nil {
		return err
	}
	defer pool.Close()
	return run(ctx, momo.New(pgxdriver.New(pool)), os.Stdout)
}

// run walks through every step and prints what it finds. The test compares
// the output, so the printed lines are the contract of this example.
func run(ctx context.Context, db *momo.DB, out io.Writer) error {
	steps := []func(context.Context, momo.Querier, io.Writer) error{
		seed,
		project,
		paginate,
		count,
	}
	for _, step := range steps {
		if err := step(ctx, db, out); err != nil {
			return err
		}
	}
	return nil
}

// seed inserts eight articles. Several share a score on purpose: ties are
// what makes naive pagination skip or repeat rows.
func seed(ctx context.Context, q momo.Querier, _ io.Writer) error {
	scores := []int64{5, 5, 5, 4, 4, 3, 3, 1}
	rows := make([]models.Article, len(scores))
	for i, s := range scores {
		title := string(rune('a' + i))
		rows[i] = models.Article{Title: title, Body: strings.Repeat(title, 1000), Score: s}
	}
	_, err := momo.Insert[models.Article](q).Models(rows).Exec(ctx)
	return err
}

// cards starts a query for the list view: the Card projection, read from the
// articles table, without the large Body column. Columns lists the columns in
// the field order of Card, and the compiler checks each one against its Go
// type through the generated article package.
func cards(q momo.Querier) momo.SelectQuery[models.Card] {
	return momo.Select[models.Card](q).
		From(article.Table).
		Columns(article.ID, article.Title, article.Score)
}

// project reads a subset of columns into a smaller struct. The statement
// never selects body, so the database does not send it.
func project(ctx context.Context, q momo.Querier, out io.Writer) error {
	query := cards(q).
		Where(article.Score.Ge(4)).
		OrderBy(article.Score.Desc(), article.ID.Asc())
	compiled, err := query.SQL()
	if err != nil {
		return err
	}
	fmt.Fprintln(out, "sql:", compiled.SQL)
	list, err := query.All(ctx)
	if err != nil {
		return err
	}
	for _, c := range list {
		fmt.Fprintf(out, "card %s score=%d\n", c.Title, c.Score)
	}
	return nil
}

// cursor is the position of the last row of a page: the value of every ORDER
// BY key of that row. Hand it to the client as an opaque token.
type cursor struct{ Score, ID int64 }

// page returns up to size cards after the cursor (nil: the first page) and
// the cursor of the next page (nil: this was the last one).
//
// The order is score descending, then id ascending. The id is the
// tie-breaker: scores repeat, ids do not, so every row has exactly one place
// in the order and no page skips or repeats a row, even when rows are
// inserted between requests. A keyset query with a non-unique order is a bug.
//
// After takes one value per ORDER BY key, in order. For a model it appends
// the primary key by itself; a projection has none in its metadata, so the id
// is listed in OrderBy explicitly and the cursor carries it.
//
// Fetching size+1 rows tells whether another page exists without a COUNT.
func page(ctx context.Context, q momo.Querier, after *cursor, size int64) ([]models.Card, *cursor, error) {
	query := cards(q).OrderBy(article.Score.Desc(), article.ID.Asc())
	if after != nil {
		query = query.After(after.Score, after.ID)
	}
	list, err := query.Limit(size + 1).All(ctx)
	if err != nil {
		return nil, nil, err
	}
	if int64(len(list)) <= size {
		return list, nil, nil
	}
	list = list[:size]
	last := list[size-1]
	return list, &cursor{Score: last.Score, ID: last.ID}, nil
}

// paginate walks the whole table three cards at a time.
func paginate(ctx context.Context, q momo.Querier, out io.Writer) error {
	var after *cursor
	for n := 1; ; n++ {
		list, next, err := page(ctx, q, after, 3)
		if err != nil {
			return err
		}
		titles := make([]string, len(list))
		for i, c := range list {
			titles[i] = fmt.Sprintf("%s(%d)", c.Title, c.Score)
		}
		fmt.Fprintf(out, "page %d: %s\n", n, strings.Join(titles, " "))
		if next == nil {
			return nil
		}
		after = next
	}
}

// count shows the three ways to count. Count runs SELECT COUNT(*) over the
// filtered query and ignores ordering and the cursor; Exists is Count with a
// limit of one; an aggregate projection counts per group.
func count(ctx context.Context, q momo.Querier, out io.Writer) error {
	total, err := momo.Select[models.Article](q).Count(ctx)
	if err != nil {
		return err
	}
	high, err := momo.Select[models.Article](q).Where(article.Score.Ge(4)).Count(ctx)
	if err != nil {
		return err
	}
	any5, err := momo.Select[models.Article](q).Where(article.Score.Gt(5)).Exists(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "count: total=%d score>=4: %d any above 5: %t\n", total, high, any5)

	// Scores shared by more than one article, with how many share each.
	groups, err := momo.Select[models.ScoreCount](q).
		From(article.Table).
		Columns(article.Score, momo.CountAll()).
		GroupBy(article.Score).
		Having(momo.CountAll().Gt(1)).
		OrderBy(article.Score.Desc()).
		All(ctx)
	if err != nil {
		return err
	}
	for _, g := range groups {
		fmt.Fprintf(out, "score %d: %d articles\n", g.Score, g.Articles)
	}
	return nil
}
