// SPDX-License-Identifier: Apache-2.0

//go:build integration

package vecsearch_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/pgxdriver"
	"github.com/azrtydxb/momo/internal/testdb"
	"github.com/azrtydxb/momo/vecsearch"
)

func mustExecPG(t *testing.T, q momo.Querier, sql string) {
	t.Helper()
	if _, err := momo.Exec(context.Background(), q, sql); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

// singleConnPool returns a pool with one connection, so every transaction reuses the same backend.
func singleConnPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(testdb.PostgresURL(t))
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// checkSetting raises an error unless the named setting has the expected value.
func checkSetting(ctx context.Context, q momo.Querier, name, want string) error {
	_, err := momo.Exec(ctx, q, "DO $$ BEGIN IF current_setting('"+name+"') <> '"+want+"' THEN RAISE EXCEPTION 'setting "+name+" is not "+want+"'; END IF; END $$")
	return err
}

func TestSearchSettingsScoped(t *testing.T) {
	ctx := context.Background()
	db := momo.New(pgxdriver.New(singleConnPool(t)))
	mustExecPG(t, db, "CREATE EXTENSION IF NOT EXISTS vector")
	mustExecPG(t, db, "CREATE TABLE docs (id bigint PRIMARY KEY, emb vector(4))")
	mustExecPG(t, db, "CREATE INDEX docs_emb ON docs USING hnsw (emb vector_l2_ops)")
	err := vecsearch.With(ctx, db, vecsearch.Settings{HNSWEfSearch: 200}, func(tx *momo.Tx) error {
		return checkSetting(ctx, tx, "hnsw.ef_search", "200")
	})
	if err != nil {
		t.Fatalf("inside WithSearch: %v", err)
	}
	if err := checkSetting(ctx, db, "hnsw.ef_search", "40"); err != nil {
		t.Fatalf("setting leaked after the transaction: %v", err)
	}
}

// hitCount copies the rows of table into a temporary sink and returns how many were copied.
func hitCount(ctx context.Context, tx *momo.Tx, table string) (int64, error) {
	if _, err := momo.Exec(ctx, tx, "CREATE TEMP TABLE sink_"+table+" (id bigint)"); err != nil {
		return 0, err
	}
	return momo.Exec(ctx, tx, "INSERT INTO sink_"+table+" SELECT id FROM "+table)
}

func TestFilteredANNIterativeScan(t *testing.T) {
	ctx := context.Background()
	db := momo.New(pgxdriver.New(singleConnPool(t)))
	for _, stmt := range []string{
		"CREATE EXTENSION IF NOT EXISTS vector",
		// A parallel HNSW build inserts in a nondeterministic order, which gave
		// a different graph (and a different recall) from run to run. One worker
		// with the fixed seed below builds the same graph every time.
		"SET max_parallel_maintenance_workers = 0",
		// No primary key on purpose: with a btree on id the planner may scan it
		// in full and sort exactly, which returns 10 rows with the setting off.
		// The HNSW index is then the only index path left.
		"CREATE TABLE items (id bigint NOT NULL, grp text NOT NULL, emb vector(8) NOT NULL)",
		"SELECT setseed(0.42)",
		// 4% of the rows match the filter (800 of 20000). That is sized from two
		// sides, because HNSW is approximate and the graph differs between builds
		// (it returned 9 rows with the setting on, and 10 with it off, when
		// every row had the same vector, see below): the plain scan examines ef_search = 40 candidates, so it finds about
		// 1.6 matches and reaching 10 has a probability near 1e-6, so "off returns
		// fewer than 10" holds on every build; the iterative scan keeps walking
		// the graph until it has 10, and with 800 matches spread uniformly at
		// random an unreachable or poorly connected region cannot hide all but 9
		// of them. Both runs keep the default ef_search so the only difference
		// between them is the iterative scan.
		// The "+ g * 0" ties the vector subquery to the row. Without it the
		// planner evaluates the uncorrelated ARRAY(SELECT random()...) once and
		// every row gets the same vector, a degenerate graph that no recall
		// argument applies to.
		"INSERT INTO items SELECT g, CASE WHEN g % 25 = 0 THEN 'rare' ELSE 'common' END, ARRAY(SELECT (random() + g * 0)::real FROM generate_series(1, 8))::vector FROM generate_series(1, 20000) AS g",
		"CREATE INDEX items_emb ON items USING hnsw (emb vector_l2_ops)",
		"ANALYZE items",
	} {
		mustExecPG(t, db, stmt)
	}
	count := func(name string, s vecsearch.Settings) int64 {
		var n int64
		err := vecsearch.With(ctx, db, s, func(tx *momo.Tx) error {
			// The test is about the HNSW scan, so the planner must not choose an
			// exact sequential scan (which returns 10 rows with the setting off).
			for _, stmt := range []string{"SET LOCAL enable_seqscan = off", "SET LOCAL enable_bitmapscan = off"} {
				if _, err := momo.Exec(ctx, tx, stmt); err != nil {
					return err
				}
			}
			const query = "SELECT id FROM items WHERE grp = 'rare' ORDER BY emb <-> (SELECT emb FROM items WHERE id = 1) LIMIT 10"
			// Fail clearly, not flakily, when the plan is not the HNSW index scan.
			assertPlan := "DO $do$ DECLARE r text; ok boolean := false; BEGIN FOR r IN EXECUTE $q$EXPLAIN " + query + "$q$ LOOP IF r LIKE '%Index Scan using items_emb%' THEN ok := true; END IF; END LOOP; IF NOT ok THEN RAISE EXCEPTION 'plan does not use the items_emb HNSW index'; END IF; END $do$"
			if _, err := momo.Exec(ctx, tx, assertPlan); err != nil {
				return err
			}
			if _, err := momo.Exec(ctx, tx, "CREATE TEMP TABLE "+name+" AS "+query); err != nil {
				return err
			}
			var err error
			n, err = hitCount(ctx, tx, name)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	off := count("hits_off", vecsearch.Settings{})
	on := count("hits_on", vecsearch.Settings{HNSWIterativeScan: vecsearch.IterativeRelaxedOrder, HNSWMaxScanTuples: 1_000_000})
	if off >= 10 {
		t.Fatalf("iterative scan off returned %d rows, want fewer than 10", off)
	}
	if on != 10 {
		t.Fatalf("iterative scan on returned %d rows, want 10", on)
	}
	if err := checkSetting(ctx, db, "hnsw.iterative_scan", "off"); err != nil {
		t.Fatalf("setting leaked after the transaction: %v", err)
	}
}
