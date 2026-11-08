// SPDX-License-Identifier: Apache-2.0

// Package stats computes the latency percentiles the benchmarks report.
package stats

import (
	"math"
	"slices"
	"testing"
	"time"
)

// Percentile returns the nearest-rank percentile p (0 < p <= 100) of samples.
// The samples are copied, so the caller's slice keeps its order. An empty
// slice yields 0.
func Percentile(samples []time.Duration, p float64) time.Duration {
	if len(samples) == 0 {
		return 0
	}
	sorted := slices.Clone(samples)
	slices.Sort(sorted)
	idx := int(math.Ceil(p/100*float64(len(sorted)))) - 1
	idx = max(0, min(idx, len(sorted)-1))
	return sorted[idx]
}

// Report sets p50_ns, p95_ns and p99_ns metrics on b from samples collected
// during the timed loop.
func Report(b *testing.B, samples []time.Duration) {
	b.Helper()
	b.ReportMetric(float64(Percentile(samples, 50).Nanoseconds()), "p50_ns")
	b.ReportMetric(float64(Percentile(samples, 95).Nanoseconds()), "p95_ns")
	b.ReportMetric(float64(Percentile(samples, 99).Nanoseconds()), "p99_ns")
}
