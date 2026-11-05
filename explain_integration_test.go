// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/vector"
)

func TestVectorIndexUsed(t *testing.T) {
	ctx := context.Background()
	db := testopen.Postgres(t)
	mustExecPG(t, db, "CREATE EXTENSION IF NOT EXISTS vector")
	mustExecPG(t, db, "DROP TABLE IF EXISTS docs")
	mustExecPG(t, db, "CREATE TABLE docs (id bigint PRIMARY KEY, emb vector(8))")
	mustExecPG(t, db, "CREATE INDEX docs_emb_hnsw ON docs USING hnsw (emb vector_l2_ops)")

	// Fixed PCG seed: the same 5000 vectors and the same query on every run.
	rng := rand.New(rand.NewPCG(11, 11))
	randVec := func() vector.Vector {
		v := make(vector.Vector, 8)
		for i := range v {
			v[i] = rng.Float32()
		}
		return v
	}
	if err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
		for i := 1; i <= 5000; i++ {
			if _, err := momo.Exec(ctx, tx, "INSERT INTO docs (id, emb) VALUES (?, ?)", int64(i), randVec()); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	mustExecPG(t, db, "ANALYZE docs")

	col := momo.NewVectorColumn[vector.Vector]("docs", "emb", 8, 0)
	query := randVec()

	// checkPlan runs EXPLAIN on the KNN query. enable_seqscan is off for this
	// transaction only (set_config(..., true)), so the planner must pick an index.
	checkPlan := func(t *testing.T, indexName string) {
		t.Helper()
		var plan string
		if err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
			if err := tx.SetLocal(ctx, "enable_seqscan", "off"); err != nil {
				return err
			}
			var err error
			plan, err = momo.Select[vecDoc, *vecDoc](tx).
				OrderBy(momo.L2Distance(col, query).Asc()).
				Limit(10).
				Explain(ctx)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(plan, "\n") {
			if strings.Contains(line, indexName) {
				return
			}
		}
		t.Fatalf("EXPLAIN does not use index %s:\n%s", indexName, plan)
	}

	checkPlan(t, "docs_emb_hnsw")

	t.Run("IVFFlat", func(t *testing.T) {
		mustExecPG(t, db, "DROP INDEX docs_emb_hnsw")
		mustExecPG(t, db, "CREATE INDEX docs_emb_ivf ON docs USING ivfflat (emb vector_l2_ops) WITH (lists = 10)")
		mustExecPG(t, db, "ANALYZE docs")
		checkPlan(t, "docs_emb_ivf")
	})
}
