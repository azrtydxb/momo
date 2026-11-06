// SPDX-License-Identifier: Apache-2.0

package dialect

import (
	"slices"
	"testing"
)

func TestSQLiteVectorCaps(t *testing.T) {
	caps := For(SQLite).Vector
	want := []Metric{MetricL2, MetricL1, MetricCosine, MetricHamming}
	if len(caps.Metrics) != len(want) {
		t.Fatalf("sqlite metrics = %v, want %v", caps.Metrics, want)
	}
	for _, m := range want {
		if !slices.Contains(caps.Metrics, m) {
			t.Errorf("sqlite metric %s missing", m)
		}
	}
	if slices.Contains(caps.Metrics, MetricInnerProduct) || slices.Contains(caps.Metrics, MetricJaccard) {
		t.Errorf("sqlite must not advertise ip or jaccard: %v", caps.Metrics)
	}
	if len(caps.Indexes) != 0 {
		t.Errorf("sqlite ANN indexes = %v, want none", caps.Indexes)
	}
}

// TestVectorCapsMatrix checks every metric on every dialect.
func TestVectorCapsMatrix(t *testing.T) {
	all := []Metric{MetricL2, MetricInnerProduct, MetricCosine, MetricL1, MetricHamming, MetricJaccard}
	cases := []struct {
		name    Name
		storage bool
		metrics []Metric
		indexes []string
	}{
		{Postgres, true, all, []string{"hnsw", "ivfflat"}},
		{SQLite, true, []Metric{MetricL2, MetricL1, MetricCosine, MetricHamming}, nil},
		{MySQL, false, nil, nil},
	}
	for _, c := range cases {
		caps := For(c.name).Vector
		if caps.Storage != c.storage {
			t.Errorf("%s storage = %v, want %v", c.name, caps.Storage, c.storage)
		}
		for _, m := range all {
			if got, want := slices.Contains(caps.Metrics, m), slices.Contains(c.metrics, m); got != want {
				t.Errorf("%s metric %s supported = %v, want %v", c.name, m, got, want)
			}
		}
		if !slices.Equal(caps.Indexes, c.indexes) {
			t.Errorf("%s indexes = %v, want %v", c.name, caps.Indexes, c.indexes)
		}
	}
}
