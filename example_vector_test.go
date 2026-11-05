// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/driver/pgxdriver"
	"github.com/azrtydxb/momo/internal/testmodels/example"
	"github.com/azrtydxb/momo/migrate"
	migratepg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/schema"
	"github.com/azrtydxb/momo/vector"
)

// docEmbedding is the typed descriptor of documents.embedding. Generated
// code emits a plain momo.Column for a vector field, so the descriptor that
// carries the dimension is built by hand with NewVectorColumn.
var docEmbedding = momo.NewVectorColumn[vector.Vector]("documents", "embedding", 1536, 0)

// A nearest-neighbour query orders by a typed distance expression. The query
// vector is bound as an argument and checked against the column dimension
// when the SQL is built.
func ExampleCosineDistance() {
	// SQL() only compiles the statement, so no connection is needed here.
	db := momo.New(pgxdriver.New(nil))
	query := make(vector.Vector, 1536)
	query[0] = 1

	c, err := momo.Select[example.Document, *example.Document](db).
		OrderBy(momo.CosineDistance(docEmbedding, query).Asc()).
		Limit(5).
		SQL()
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(c.SQL)
	// Output:
	// SELECT "documents"."id", "documents"."tenant_id", "documents"."title", "documents"."body", "documents"."embedding", "documents"."status", "documents"."created_at" FROM "documents" ORDER BY ("documents"."embedding" <=> $1) ASC LIMIT $2
}

// A query vector of the wrong dimension is a build error, not a server error.
func ExampleL2Distance() {
	db := momo.New(pgxdriver.New(nil))

	_, err := momo.Select[example.Document, *example.Document](db).
		OrderBy(momo.L2Distance(docEmbedding, vector.Vector{1, 2, 3}).Asc()).
		SQL()
	fmt.Println(err != nil)
	// Output:
	// true
}

// The metrics and index methods each dialect supports come from
// dialect.Dialect.Vector. A distance the dialect lacks fails at build time
// with ErrUnsupported.
func ExampleVectorColumn() {
	for _, name := range []dialect.Name{dialect.Postgres, dialect.SQLite} {
		caps := dialect.For(name).Vector
		fmt.Println(name, caps.Metrics, caps.Indexes)
	}
	// Output:
	// postgres [l2 ip cosine l1 hamming jaccard] [hnsw ivfflat]
	// sqlite [l2 l1 cosine hamming] []
}

// Bit vectors take the Hamming and Jaccard distances.
func ExampleHammingDistance() {
	db := momo.New(pgxdriver.New(nil))
	col := momo.NewVectorColumn[vector.Bits]("documents", "bits", 8, 0)

	expr := momo.HammingDistance(col, vector.Bits{Len: 8, Data: []byte{0xF0}})
	c, err := momo.Select[example.Document, *example.Document](db).
		OrderBy(expr.Asc()).Limit(3).SQL()
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(strings.Contains(c.SQL, `("documents"."bits" <~> $1) ASC`))
	// Output:
	// true
}

// SearchSettings are applied with set_config(..., is_local = true), so they
// end with the transaction and never reach a pooled connection.
func ExampleDB_WithSearch() {
	var db *momo.DB // opened with momo.New(pgxdriver.New(pool))
	ctx := context.Background()

	settings := momo.SearchSettings{
		HNSWEfSearch:      100,
		HNSWIterativeScan: momo.IterativeRelaxedOrder, // needs pgvector 0.8
		HNSWMaxScanTuples: 20000,
	}
	err := db.WithSearch(ctx, settings, func(tx *momo.Tx) error {
		_, err := momo.Select[example.Document, *example.Document](tx).
			OrderBy(momo.CosineDistance(docEmbedding, make(vector.Vector, 1536)).Asc()).
			Limit(10).
			All(ctx)
		return err
	})
	fmt.Println(err)
}

// ExactSearch bypasses the vector index for the rest of the transaction so
// the result is the exact nearest neighbours.
func ExampleTx_SetSearch() {
	var db *momo.DB
	ctx := context.Background()

	err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
		if err := tx.SetSearch(ctx, momo.SearchSettings{ExactSearch: true}); err != nil {
			return err
		}
		// Queries here scan the table. Keep this transaction short: the
		// planner switches also affect every other index lookup in it.
		return nil
	})
	fmt.Println(err)
}

// Explain returns the plan text without executing the query, so a test can
// check that a vector query uses its index.
func ExampleSelectQuery_Explain() {
	var db *momo.DB
	ctx := context.Background()

	plan, err := momo.Select[example.Document, *example.Document](db).
		OrderBy(momo.L2Distance(docEmbedding, make(vector.Vector, 1536)).Asc()).
		Limit(10).
		Explain(ctx)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(plan) // look for "Index Scan using documents_embedding_hnsw"
}

// ValidateVectorIndex checks the column type, operator class and storage
// parameters before any SQL runs.
func ExampleValidateVectorIndex() {
	tbl := &schema.Table{Name: "documents", Columns: []schema.Column{{Name: "embedding", Type: "vector(1536)"}}}

	good := schema.Index{
		Name: "documents_embedding_hnsw", Method: "hnsw",
		Parts: []schema.IndexPart{{Column: "embedding", OpClass: "vector_cosine_ops"}},
		With:  []schema.KV{{Key: "m", Value: "16"}, {Key: "ef_construction", Value: "64"}},
	}
	fmt.Println(schema.ValidateVectorIndex(dialect.Postgres, tbl, good))

	noOpClass := good
	noOpClass.Parts = []schema.IndexPart{{Column: "embedding"}}
	fmt.Println(schema.ValidateVectorIndex(dialect.Postgres, tbl, noOpClass))

	op, _ := schema.VectorIndexOpClass("halfvec(8)", dialect.MetricL1)
	fmt.Println(op)
	// Output:
	// <nil>
	// schema: invalid vector index: index "documents_embedding_hnsw": hnsw on vector has no default operator class, declare one such as vector_l2_ops
	// halfvec_l1_ops
}

// A migration creates the extension, the table and the ANN index from the
// schema description.
func Example_vectorMigration() {
	to := &schema.Schema{
		Dialect:    dialect.Postgres,
		Extensions: []schema.Extension{{Name: "vector"}},
		Tables: []schema.Table{{
			Name: "documents",
			Columns: []schema.Column{
				{Name: "id", Type: "bigint"},
				{Name: "embedding", Type: "vector(3)"},
			},
			PrimaryKey: []string{"id"},
			Indexes: []schema.Index{{
				Name: "documents_embedding_hnsw", Method: "hnsw",
				Parts: []schema.IndexPart{{Column: "embedding", OpClass: "vector_cosine_ops"}},
				With:  []schema.KV{{Key: "m", Value: "16"}, {Key: "ef_construction", Value: "64"}},
			}},
		}},
	}
	changes, _, err := migrate.Diff(nil, to, migrate.DiffOptions{})
	if err != nil {
		fmt.Println(err)
		return
	}
	stmts, err := migratepg.New().Render(migrate.Order(changes))
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, s := range stmts {
		fmt.Println(s.SQL)
	}
	// Output:
	// CREATE EXTENSION IF NOT EXISTS "vector"
	// CREATE TABLE "documents" ("id" bigint NOT NULL, "embedding" vector(3) NOT NULL, CONSTRAINT "documents_pkey" PRIMARY KEY ("id"))
	// CREATE INDEX "documents_embedding_hnsw" ON "documents" USING "hnsw" ("embedding" "vector_cosine_ops") WITH ("m" = 16, "ef_construction" = 64)
}

// The binary pgvector codec is opt-in: ConfigureVectors registers it on the
// pool, WithVectorFormat chooses it for the driver.
func ExampleConfigureVectors() {
	cfg, err := pgxpool.ParseConfig("postgres://localhost/app")
	if err != nil {
		fmt.Println(err)
		return
	}
	pgxdriver.ConfigureVectors(cfg)
	// pool, err := pgxpool.NewWithConfig(ctx, cfg)
	_ = pgxdriver.New(nil, pgxdriver.WithVectorFormat(pgxdriver.VectorBinary))
	fmt.Println(cfg.AfterConnect != nil)
	// Output:
	// true
}
