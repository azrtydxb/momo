// SPDX-License-Identifier: Apache-2.0

// Command vector-search shows nearest-neighbour search with pgvector: the
// column and HNSW index (see models/models.go and the migration), a query
// ordered by a distance, filters, a limit, and per-search settings.
//
// Run it against a database that has the migrations applied (the first
// migration runs CREATE EXTENSION vector, which needs a role allowed to):
//
//	DATABASE_URL=postgres://... go run ./examples/vector-search
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
	"github.com/azrtydxb/momo/examples/vector-search/models"
	"github.com/azrtydxb/momo/examples/vector-search/models/item"
	"github.com/azrtydxb/momo/vecsearch"
	"github.com/azrtydxb/momo/vector"
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

// query is the embedding of the search text. A real program gets it from the
// same embedding model that produced the stored vectors.
var query = vector.Vector{1, 0.2, 0}

// run walks through every step and prints what it finds. The test compares
// the output, so the printed lines are the contract of this example.
func run(ctx context.Context, db *momo.DB, out io.Writer) error {
	if err := seed(ctx, db); err != nil {
		return err
	}
	for _, step := range []func(context.Context, *momo.DB, io.Writer) error{
		nearest,
		nearestWithFilter,
		searchSettings,
		explainIndex,
	} {
		if err := step(ctx, db, out); err != nil {
			return err
		}
	}
	return nil
}

func seed(ctx context.Context, q momo.Querier) error {
	_, err := momo.Insert[models.Item](q).Models([]models.Item{
		{Title: "engine notes", Category: "news", Embedding: vector.Vector{1, 0, 0}},
		{Title: "engine repair", Category: "howto", Embedding: vector.Vector{0.9, 0.1, 0}},
		{Title: "cooking basics", Category: "howto", Embedding: vector.Vector{0, 1, 0}},
		{Title: "engine history", Category: "news", Embedding: vector.Vector{0.7, 0.7, 0}},
		{Title: "garden tips", Category: "howto", Embedding: vector.Vector{0, 0, 1}},
		{Title: "engine launch", Category: "news", Embedding: vector.Vector{0.8, 0, 0.6}},
	}).Exec(ctx)
	return err
}

// nearest orders by the cosine distance to the query, nearest first, and keeps
// the best three. The distance builder matches the index operator class
// (vector_cosine_ops), so PostgreSQL can answer the ORDER BY from the HNSW
// index. The query vector is a bound argument, and a vector of the wrong
// dimension fails when the SQL is built, before any round trip.
//
// The select list holds the distance too, which needs the Hit projection. If
// you only need the rows, select the model (momo.Select[models.Item]) and
// leave the distance out.
func nearest(ctx context.Context, db *momo.DB, out io.Writer) error {
	hits, err := search(ctx, db, 3)
	if err != nil {
		return err
	}
	printHits(out, "nearest", hits)
	return nil
}

// search is the reusable query. Extra predicates go in where; Where can only
// narrow the candidates, it never reorders them.
//
// The order among equal distances is unspecified. Adding a tie-break key such
// as id makes PostgreSQL sort on top of the index scan, and on some versions
// (seen on PG16) the planner then drops the HNSW index.
func search(ctx context.Context, q momo.Querier, limit int64, where ...momo.Predicate) ([]models.Hit, error) {
	return searchQuery(q, limit, where...).All(ctx)
}

// searchQuery builds the query without running it, so Explain plans exactly
// the statement search runs.
func searchQuery(q momo.Querier, limit int64, where ...momo.Predicate) momo.SelectQuery[models.Hit] {
	distance := momo.CosineDistance(item.Embedding, query)
	return momo.Select[models.Hit](q).
		From(item.Table).
		Columns(item.Title, distance).
		Where(where...).
		OrderBy(distance.Asc()).
		Limit(limit)
}

// explainIndex checks that the query uses the HNSW index. Explain runs EXPLAIN
// (never EXPLAIN ANALYZE), so nothing executes. The planner prefers a
// sequential scan on a table this small, so the transaction turns sequential
// scans off with SetLocal (reverts at commit); in production, with enough
// rows, the planner picks the index by itself and you leave this out. The
// plan text varies between PostgreSQL versions, so the example tests for the
// index name instead of printing the plan.
// The explained plan is the query search runs.
func explainIndex(ctx context.Context, db *momo.DB, out io.Writer) error {
	return db.Tx(ctx, momo.TxOptions{ReadOnly: true}, func(tx *momo.Tx) error {
		if err := tx.SetLocal(ctx, "enable_seqscan", "off"); err != nil {
			return err
		}
		plan, err := searchQuery(tx, 3).Explain(ctx)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "plan uses items_embedding_hnsw: %t\n", strings.Contains(plan, "items_embedding_hnsw"))
		return nil
	})
}

// nearestWithFilter combines similarity with an ordinary filter.
//
// Know the trap: an HNSW index returns its nearest candidates first and the
// filter is applied to them afterwards. With a selective filter that can
// leave fewer than the limit, even when enough matching rows exist. Iterative
// scan (pgvector 0.8 or newer) fixes it: the index keeps scanning until
// enough rows pass the filter. relaxed_order returns slightly out-of-order
// results for more speed; strict_order keeps the order exactly.
//
// vecsearch.With applies the settings to one transaction only (they end at commit
// or rollback), so they never leak to other users of a pooled connection and
// are safe behind PgBouncer in transaction mode.
func nearestWithFilter(ctx context.Context, db *momo.DB, out io.Writer) error {
	settings := vecsearch.Settings{
		HNSWIterativeScan: vecsearch.IterativeRelaxedOrder,
		HNSWMaxScanTuples: 20000,
	}
	return vecsearch.With(ctx, db, settings, func(tx *momo.Tx) error {
		hits, err := search(ctx, tx, 2, item.Category.Eq("news"))
		if err != nil {
			return err
		}
		printHits(out, "nearest news", hits)
		return nil
	})
}

// searchSettings shows the two knobs that trade speed for recall.
//
// HNSWEfSearch is how many candidates the index keeps while it searches
// (default 40). Raise it for better recall at the cost of latency; it must be
// at least the limit. IVFFlat has the matching IVFFlatProbes.
//
// ExactSearch switches the index off for the transaction, so the search scans
// the table and returns the true nearest neighbours. Use it to measure the
// recall of the index, or for small tables. It also turns off every other
// index in that transaction, so keep that transaction short.
func searchSettings(ctx context.Context, db *momo.DB, out io.Writer) error {
	for _, s := range []struct {
		label    string
		settings vecsearch.Settings
	}{
		{"ef_search 100", vecsearch.Settings{HNSWEfSearch: 100}},
		{"exact", vecsearch.Settings{ExactSearch: true}},
	} {
		err := vecsearch.With(ctx, db, s.settings, func(tx *momo.Tx) error {
			hits, err := search(ctx, tx, 1)
			if err != nil {
				return err
			}
			printHits(out, s.label, hits)
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func printHits(out io.Writer, label string, hits []models.Hit) {
	parts := make([]string, len(hits))
	for i, h := range hits {
		parts[i] = fmt.Sprintf("%s (%.3f)", h.Title, h.Distance)
	}
	fmt.Fprintf(out, "%s: %s\n", label, strings.Join(parts, ", "))
}
