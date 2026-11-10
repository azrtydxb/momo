// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/vector"
)

func hybridIntegrationSpec() momo.HybridSpec {
	s := hybridSpec()
	s.Query = "vector search"
	s.Vector = momo.NewVectorColumn[vector.Vector]("docs", "emb", 4, 0)
	s.QueryVector = vector.Vector{1, 0, 0, 0}
	return s
}

// setupHybrid creates a fresh schema with the docs table and the given rows.
func setupHybrid(t *testing.T, rows ...hybridDoc) *momo.DB {
	t.Helper()
	db := testopen.Postgres(t)
	mustExecPG(t, db, "CREATE EXTENSION IF NOT EXISTS vector")
	mustExecPG(t, db, "CREATE TABLE docs (id bigint PRIMARY KEY, body text NOT NULL, emb vector(4) NOT NULL)")
	for _, r := range rows {
		mustExecPG(t, db, "INSERT INTO docs (id, body, emb) VALUES (?, ?, ?)", r.ID, r.Body, r.Emb)
	}
	return db
}

func runHybrid(t *testing.T, q momo.Querier, spec momo.HybridSpec) []momo.HybridResult[hybridDoc] {
	t.Helper()
	res, err := momo.HybridSearch[hybridDoc](q, spec).All(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func hybridIDs(res []momo.HybridResult[hybridDoc]) []int64 {
	ids := make([]int64, 0, len(res))
	for _, r := range res {
		ids = append(ids, r.Row.ID)
	}
	return ids
}

// Catches: the fusion ordering (RRF k=60, default 100 candidates per leg),
// dependence on insertion order, a tie on the fused score not breaking on the
// primary key, and the candidate counts not limiting a leg.
func TestHybridSearch(t *testing.T) {
	// "vector search" matches only id 1 as text (websearch_to_tsquery ANDs the
	// terms). The vector leg ranks 1 and 3 (distance 0, tie broken on id), then
	// 2 and 4 (distance 1). Scores: 1 is 2/61, 3 is 1/62, 2 is 1/63, 4 is 1/64.
	corpus := []hybridDoc{
		{ID: 1, Body: "vector search", Emb: vector.Vector{1, 0, 0, 0}},
		{ID: 2, Body: "vector", Emb: vector.Vector{0, 1, 0, 0}},
		{ID: 3, Body: "search", Emb: vector.Vector{1, 0, 0, 0}},
		{ID: 4, Body: "unrelated words", Emb: vector.Vector{0, 0, 1, 0}},
	}
	want := []int64{1, 3, 2, 4}
	res := runHybrid(t, setupHybrid(t, corpus...), hybridIntegrationSpec())
	if got := hybridIDs(res); !slices.Equal(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
	wantScores := []float64{2.0 / 61, 1.0 / 62, 1.0 / 63, 1.0 / 64}
	for i, r := range res {
		if math.Abs(r.Score-wantScores[i]) > 1e-9 {
			t.Errorf("score[%d] = %v, want %v", i, r.Score, wantScores[i])
		}
	}

	t.Run("insertion order does not change the result", func(t *testing.T) {
		rev := slices.Clone(corpus)
		slices.Reverse(rev)
		if got := hybridIDs(runHybrid(t, setupHybrid(t, rev...), hybridIntegrationSpec())); !slices.Equal(got, want) {
			t.Fatalf("order = %v, want %v", got, want)
		}
	})

	t.Run("tie on fused score breaks on primary key", func(t *testing.T) {
		// id 5 is text rank 1 and vector rank 2; id 9 is text rank 2 and
		// vector rank 1. Both fused scores are 1/61 + 1/62, so the smaller
		// primary key comes first.
		tie := []hybridDoc{
			{ID: 9, Body: "vector search", Emb: vector.Vector{1, 0, 0, 0}},
			{ID: 5, Body: "vector search", Emb: vector.Vector{0.70710678, 0.70710678, 0, 0}},
		}
		if got := hybridIDs(runHybrid(t, setupHybrid(t, tie...), hybridIntegrationSpec())); !slices.Equal(got, []int64{5, 9}) {
			t.Fatalf("tie order = %v, want [5 9]", got)
		}
	})

	t.Run("vector candidates bound the vector leg", func(t *testing.T) {
		spec := hybridIntegrationSpec()
		spec.VectorCandidates = 2 // ids 1 and 3 only
		if got := hybridIDs(runHybrid(t, setupHybrid(t, corpus...), spec)); !slices.Equal(got, []int64{1, 3}) {
			t.Fatalf("order = %v, want [1 3]", got)
		}
	})

	t.Run("limit and rank constant", func(t *testing.T) {
		spec := hybridIntegrationSpec()
		spec.Limit = 1
		spec.RRFK = 1
		res := runHybrid(t, setupHybrid(t, corpus...), spec)
		if len(res) != 1 || res[0].Row.ID != 1 || math.Abs(res[0].Score-1.0) > 1e-9 {
			t.Fatalf("result = %+v, want id 1 with score 2/(1+1) = 1", res)
		}
	})
}

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// Catches: a row found only by the text leg, or only by the vector leg,
// getting no score or the wrong one (1/(k+rank) of the one leg it is in).
func TestHybridSearchSingleLegHits(t *testing.T) {
	db := setupHybrid(t,
		hybridDoc{ID: 2, Body: "vector search", Emb: vector.Vector{0, 1, 0, 0}},
		hybridDoc{ID: 1, Body: "zzz", Emb: vector.Vector{1, 0, 0, 0}},
	)
	spec := hybridIntegrationSpec()
	spec.VectorCandidates = 1 // id 1 only; id 2 is found by the text leg alone
	res := runHybrid(t, db, spec)
	if got := hybridIDs(res); !slices.Equal(got, []int64{1, 2}) {
		t.Fatalf("order = %v, want [1 2]", got)
	}
	for _, r := range res {
		if !approx(r.Score, 1.0/61) {
			t.Errorf("id %d score = %v, want 1/61 (one leg, rank 1)", r.Row.ID, r.Score)
		}
	}
}

// Catches: TextCandidates not bounding the text leg.
func TestHybridSearchTextCandidatesBoundTextLeg(t *testing.T) {
	db := setupHybrid(t,
		hybridDoc{ID: 1, Body: "vector search", Emb: vector.Vector{0, 1, 0, 0}},
		hybridDoc{ID: 2, Body: "vector search", Emb: vector.Vector{0, 1, 0, 0}},
		hybridDoc{ID: 3, Body: "vector search", Emb: vector.Vector{0, 1, 0, 0}},
		hybridDoc{ID: 9, Body: "zzz", Emb: vector.Vector{1, 0, 0, 0}},
	)
	spec := hybridIntegrationSpec()
	spec.TextCandidates = 1 // equal text rank, so id 1 by primary key
	spec.VectorCandidates = 1
	if got := hybridIDs(runHybrid(t, db, spec)); !slices.Equal(got, []int64{1, 9}) {
		t.Fatalf("order = %v, want [1 9]", got)
	}
}

// Catches: rows with a NULL embedding or a zero vector (NaN cosine distance)
// taking vector candidates and a rank.
func TestHybridSearchSkipsNullAndNaNEmbeddings(t *testing.T) {
	db := testopen.Postgres(t)
	mustExecPG(t, db, "CREATE EXTENSION IF NOT EXISTS vector")
	mustExecPG(t, db, "CREATE TABLE docs (id bigint PRIMARY KEY, body text NOT NULL, emb vector(4))")
	mustExecPG(t, db, "INSERT INTO docs (id, body, emb) VALUES (1, 'vector search', '[1,0,0,0]'), (2, 'unrelated', NULL), (3, 'unrelated', '[0,0,0,0]'), (4, 'unrelated', '[0,1,0,0]')")
	spec := hybridIntegrationSpec()
	spec.VectorCandidates = 3
	if got := hybridIDs(runHybrid(t, db, spec)); !slices.Equal(got, []int64{1, 4}) {
		t.Fatalf("order = %v, want [1 4] (NULL and zero-vector rows must not rank)", got)
	}
}

// Catches: soft-deleted rows reaching either leg, and a schema-qualified table
// failing when executed.
func TestHybridSearchSoftDeleteAndSchemaLive(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	mustExecPG(t, db, "CREATE EXTENSION IF NOT EXISTS vector")
	mustExecPG(t, db, "DROP SCHEMA IF EXISTS hybrid_kb CASCADE")
	mustExecPG(t, db, "CREATE SCHEMA hybrid_kb")
	t.Cleanup(func() { _, _ = momo.Exec(ctx, db, "DROP SCHEMA IF EXISTS hybrid_kb CASCADE") })
	mustExecPG(t, db, "CREATE TABLE hybrid_kb.soft_docs (id bigint PRIMARY KEY, body text NOT NULL, emb vector(4) NOT NULL, deleted_at timestamptz)")
	mustExecPG(t, db, "INSERT INTO hybrid_kb.soft_docs (id, body, emb, deleted_at) VALUES (1, 'vector search', '[1,0,0,0]', NULL), (2, 'vector search', '[1,0,0,0]', now()), (3, 'other', '[1,0,0,0]', NULL)")
	spec := momo.HybridSpec{
		Text:        momo.NewColumn[string]("soft_docs", "body", 0),
		Query:       "vector search",
		TextConfig:  "english",
		Vector:      momo.NewVectorColumn[vector.Vector]("soft_docs", "emb", 4, 0),
		QueryVector: vector.Vector{1, 0, 0, 0},
		Limit:       10,
	}
	res, err := momo.HybridSearch[hybridSoftDoc](db, spec).All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for _, r := range res {
		ids = append(ids, r.Row.ID)
	}
	if !slices.Equal(ids, []int64{1, 3}) {
		t.Fatalf("ids = %v, want [1 3] (soft-deleted id 2 excluded)", ids)
	}
}

// Catches: the vector leg's ORDER BY shape stopping pgvector's hnsw index
// from serving it.
func TestHybridSearchUsesHNSWIndex(t *testing.T) {
	db := setupHybrid(t)
	ctx := context.Background()
	mustExecPG(t, db, "INSERT INTO docs SELECT g, 'word ' || g, ARRAY(SELECT random()::real FROM generate_series(1, 4))::vector FROM generate_series(1, 5000) AS g")
	mustExecPG(t, db, "CREATE INDEX docs_emb_hnsw ON docs USING hnsw (emb vector_cosine_ops)")
	mustExecPG(t, db, "ANALYZE docs")
	c, err := momo.HybridSearch[hybridDoc](db, hybridIntegrationSpec()).SQL()
	if err != nil {
		t.Fatal(err)
	}
	err = db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
		if err := tx.SetLocal(ctx, "enable_seqscan", "off"); err != nil {
			return err
		}
		lines, err := momo.Scalars[string](ctx, tx, "EXPLAIN "+c.SQL, c.Args...)
		if err != nil {
			return err
		}
		plan := strings.Join(lines, "\n")
		if !strings.Contains(plan, "docs_emb_hnsw") {
			t.Errorf("plan does not use the hnsw index:\n%s", plan)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
