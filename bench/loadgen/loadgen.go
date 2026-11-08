// SPDX-License-Identifier: Apache-2.0

// Package loadgen runs a mixed read and write workload against one
// workload.Impl with a fixed number of concurrent workers.
package loadgen

import (
	"context"
	"errors"
	"math/rand"
	"sync"
	"time"

	"github.com/azrtydxb/momo/bench/stats"
	"github.com/azrtydxb/momo/bench/workload"
)

// Config selects the shape of one run. Zero fields take the documented default.
type Config struct {
	// Concurrency is the number of goroutines; default 8.
	Concurrency int
	// Ops is the total number of operations; default 20000.
	Ops int
	// ReadRatio is the fraction of CustomerByID, RecentOrders and
	// CustomersWithOrders calls; default 0.8.
	ReadRatio float64
	// Seed drives key choice and write payloads; default 42.
	Seed int64
	// Customers is the number of seeded customers keys are drawn from;
	// default 200.
	Customers int
	// PoolWaitNS, when set, is read after the run and reported as pool_wait_ns.
	// Only the native pgx adapter has such a figure.
	PoolWaitNS func() int64
}

// Result is one JSON line of loadgen output.
type Result struct {
	Impl         string  `json:"impl"`
	Concurrency  int     `json:"concurrency"`
	Ops          int     `json:"ops"`
	ReadRatio    float64 `json:"read_ratio"`
	Throughput   float64 `json:"ops_per_second"`
	P50NS        int64   `json:"p50_ns"`
	P95NS        int64   `json:"p95_ns"`
	P99NS        int64   `json:"p99_ns"`
	Errors       int     `json:"errors"`
	PoolWaitNS   int64   `json:"pool_wait_ns"`
	StartedAtUTC string  `json:"started_at_utc"`
}

func (c *Config) defaults() {
	if c.Concurrency <= 0 {
		c.Concurrency = 8
	}
	if c.Ops <= 0 {
		c.Ops = 20000
	}
	if c.ReadRatio <= 0 {
		c.ReadRatio = 0.8
	}
	if c.Seed == 0 {
		c.Seed = 42
	}
	if c.Customers <= 0 {
		c.Customers = 200
	}
}

// Run executes cfg.Ops operations on impl from cfg.Concurrency goroutines and
// returns the throughput and latency percentiles. Operation errors are counted,
// not returned; Run itself fails only on a canceled context.
func Run(ctx context.Context, impl workload.Impl, cfg Config) (Result, error) {
	cfg.defaults()
	res := Result{
		Impl: impl.Name(), Concurrency: cfg.Concurrency, Ops: cfg.Ops, ReadRatio: cfg.ReadRatio,
		StartedAtUTC: time.Now().UTC().Format(time.RFC3339),
	}
	ops := make(chan int, cfg.Concurrency)
	samples := make([][]time.Duration, cfg.Concurrency)
	errCounts := make([]int, cfg.Concurrency)
	var wg sync.WaitGroup
	start := time.Now()
	for w := 0; w < cfg.Concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// A deterministic source per worker; not security sensitive.
			r := rand.New(rand.NewSource(cfg.Seed + int64(w))) // #nosec G404 -- benchmark key choice
			for n := range ops {
				t0 := time.Now()
				err := oneOp(ctx, impl, r, cfg, n)
				samples[w] = append(samples[w], time.Since(t0))
				if err != nil {
					errCounts[w]++
				}
			}
		}()
	}
	var ctxErr error
feed:
	for n := 0; n < cfg.Ops; n++ {
		select {
		case ops <- n:
		case <-ctx.Done():
			ctxErr = ctx.Err()
			break feed
		}
	}
	close(ops)
	wg.Wait()
	elapsed := time.Since(start)
	if ctxErr != nil {
		return res, ctxErr
	}
	var all []time.Duration
	for w := range samples {
		all = append(all, samples[w]...)
		res.Errors += errCounts[w]
	}
	if elapsed > 0 {
		res.Throughput = float64(len(all)) / elapsed.Seconds()
	}
	res.P50NS = stats.Percentile(all, 50).Nanoseconds()
	res.P95NS = stats.Percentile(all, 95).Nanoseconds()
	res.P99NS = stats.Percentile(all, 99).Nanoseconds()
	if cfg.PoolWaitNS != nil {
		res.PoolWaitNS = cfg.PoolWaitNS()
	}
	return res, nil
}

var countries = []string{"DE", "FR", "NL", "US", "GB"}

func oneOp(ctx context.Context, impl workload.Impl, r *rand.Rand, cfg Config, n int) error {
	id := int64(r.Intn(cfg.Customers) + 1)
	country := countries[r.Intn(len(countries))]
	if r.Float64() < cfg.ReadRatio {
		switch r.Intn(3) {
		case 0:
			_, err := impl.CustomerByID(ctx, id)
			return err
		case 1:
			_, err := impl.RecentOrders(ctx, country, 20)
			return err
		default:
			_, err := impl.CustomersWithOrders(ctx, country, 10)
			return err
		}
	}
	if r.Intn(2) == 0 {
		_, err := impl.InsertOrder(ctx, workload.Order{
			CustomerID: id, AmountCents: int64(n), Status: "new",
			CreatedAt: time.Unix(1_700_000_000+int64(n), 0).UTC(),
		})
		return err
	}
	status := "active"
	if r.Intn(2) == 0 {
		status = "inactive"
	}
	rows, err := impl.SetCustomerStatus(ctx, id, status)
	if err == nil && rows != 1 {
		return errors.New("loadgen: status update touched an unexpected number of rows")
	}
	return err
}
