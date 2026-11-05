// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/pgxdriver"
)

// explain returns the EXPLAIN plan text of query inside tx.
func explain(ctx context.Context, t *testing.T, tx *momo.Tx, query string) string {
	t.Helper()
	lines, err := momo.Scalars[string](ctx, tx, "EXPLAIN "+query)
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	return strings.Join(lines, "\n")
}

func TestExactSearchBypassesVectorIndex(t *testing.T) {
	ctx := context.Background()
	db := momo.New(pgxdriver.New(singleConnPool(t)))
	for _, stmt := range []string{
		"CREATE EXTENSION IF NOT EXISTS vector",
		"CREATE TABLE exact_items (id bigint PRIMARY KEY, emb vector(8) NOT NULL)",
		"SELECT setseed(0.42)",
		"INSERT INTO exact_items SELECT g, ARRAY(SELECT random()::real FROM generate_series(1, 8))::vector FROM generate_series(1, 20000) AS g",
		"CREATE INDEX exact_items_emb ON exact_items USING hnsw (emb vector_l2_ops)",
		"ANALYZE exact_items",
	} {
		mustExecPG(t, db, stmt)
	}
	const query = "SELECT id FROM exact_items ORDER BY emb <-> '[0.5,0.5,0.5,0.5,0.5,0.5,0.5,0.5]' LIMIT 10"

	// Control: with the planner steered away from a sequential scan, the
	// ORDER BY ... LIMIT query uses the hnsw index. This proves the exact
	// case below really changed the plan.
	err := db.WithSearch(ctx, momo.SearchSettings{}, func(tx *momo.Tx) error {
		if err := tx.SetLocal(ctx, "enable_seqscan", "off"); err != nil {
			return err
		}
		plan := explain(ctx, t, tx, query)
		if !strings.Contains(plan, "exact_items_emb") {
			t.Errorf("control plan does not use the hnsw index:\n%s", plan)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	err = db.WithSearch(ctx, momo.SearchSettings{ExactSearch: true}, func(tx *momo.Tx) error {
		plan := explain(ctx, t, tx, query)
		if strings.Contains(plan, "exact_items_emb") || strings.Contains(plan, "Index Scan") {
			t.Errorf("ExactSearch plan still uses an index:\n%s", plan)
		}
		if !strings.Contains(plan, "Seq Scan") {
			t.Errorf("ExactSearch plan has no sequential scan:\n%s", plan)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSearchIterativeSettingsOnInstalledPgvector(t *testing.T) {
	ctx := context.Background()
	db := momo.New(pgxdriver.New(singleConnPool(t)))
	mustExecPG(t, db, "CREATE EXTENSION IF NOT EXISTS vector")
	err := db.WithSearch(ctx, momo.SearchSettings{
		HNSWIterativeScan: momo.IterativeRelaxedOrder, HNSWMaxScanTuples: 1000,
		IVFFlatIterativeScan: momo.IterativeRelaxedOrder, IVFFlatMaxProbes: 20,
	}, func(tx *momo.Tx) error {
		return checkSetting(ctx, tx, "hnsw.max_scan_tuples", "1000")
	})
	if err != nil {
		t.Fatalf("the pinned pgvector must support iterative settings: %v", err)
	}
}

// TestSearchSettingsPgBouncer runs set_config through PgBouncer in transaction
// mode. Pooling means two statements may land on different backends, so
// absence after commit is weak evidence by itself; the strong evidence is that
// the setting is visible inside the transaction (one backend is pinned for its
// whole length) and never observed on any of many later statements.
func TestSearchSettingsPgBouncer(t *testing.T) {
	url := os.Getenv("MOMO_TEST_PGBOUNCER_URL")
	if url == "" {
		t.Skip("MOMO_TEST_PGBOUNCER_URL is not set")
	}
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	cfg.MaxConns = 4
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	db := momo.New(pgxdriver.New(pool))

	err = db.WithSearch(ctx, momo.SearchSettings{HNSWEfSearch: 321}, func(tx *momo.Tx) error {
		return checkSetting(ctx, tx, "hnsw.ef_search", "321")
	})
	if err != nil {
		t.Fatalf("inside the transaction: %v", err)
	}
	for i := 0; i < 20; i++ {
		got, err := momo.Scalars[string](ctx, db, "SELECT coalesce(current_setting('hnsw.ef_search', true), '')")
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0] == "321" {
			t.Fatalf("query %d after commit saw %v, the setting leaked through the pool", i, got)
		}
	}
}
