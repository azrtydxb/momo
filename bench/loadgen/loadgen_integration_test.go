// SPDX-License-Identifier: Apache-2.0

//go:build integration

package loadgen_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/azrtydxb/momo/bench/adapters"
	"github.com/azrtydxb/momo/bench/adapters/pgxhand"
	"github.com/azrtydxb/momo/bench/loadgen"
	"github.com/azrtydxb/momo/bench/workload"
)

func TestLoadgenOutputSchema(t *testing.T) {
	ctx := context.Background()
	url := os.Getenv("BENCH_PG_URL")
	if url == "" {
		t.Skip("BENCH_PG_URL is not set")
	}
	pool, err := adapters.NewPool(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := workload.Seed(ctx, pool, 200, 5); err != nil {
		t.Fatal(err)
	}
	res, err := loadgen.Run(ctx, pgxhand.New(pool), loadgen.Config{Ops: 200, Concurrency: 4})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"impl", "ops_per_second", "p50_ns", "p95_ns", "p99_ns", "errors", "started_at_utc"} {
		if _, ok := fields[key]; !ok {
			t.Errorf("output lacks key %q: %s", key, raw)
		}
	}
	if res.Errors != 0 {
		t.Fatalf("errors = %d, want 0", res.Errors)
	}
	if res.Ops != 200 || res.Throughput <= 0 {
		t.Fatalf("ops = %d, throughput = %v", res.Ops, res.Throughput)
	}
}
