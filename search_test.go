// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"errors"
	"slices"
	"testing"
)

func TestSearchSettingsValidation(t *testing.T) {
	bad := []SearchSettings{
		{HNSWEfSearch: 1001},
		{HNSWEfSearch: -1},
		{HNSWIterativeScan: "sometimes"},
		{IVFFlatIterativeScan: IterativeStrictOrder}, // ivfflat has no strict_order
		{HNSWMaxScanTuples: -5},
		{IVFFlatProbes: -1},
		{IVFFlatMaxProbes: -1},
		{IVFFlatProbes: 32769},
		{IVFFlatMaxProbes: 32769},
		{HNSWMaxScanTuples: 2147483648},
	}
	for _, s := range bad {
		if err := s.Validate(); !errors.Is(err, ErrValidation) {
			t.Errorf("%+v: err = %v, want ErrValidation", s, err)
		}
	}
	good := []SearchSettings{
		{},
		{HNSWEfSearch: 1000, HNSWIterativeScan: IterativeStrictOrder, HNSWMaxScanTuples: 2147483647},
		{IVFFlatProbes: 32768, IVFFlatMaxProbes: 32768},
		{IVFFlatIterativeScan: IterativeRelaxedOrder, IVFFlatProbes: 3, IVFFlatMaxProbes: 9},
	}
	for _, s := range good {
		if err := s.Validate(); err != nil {
			t.Errorf("%+v: err = %v, want nil", s, err)
		}
	}
}

func TestSearchSQL(t *testing.T) {
	got := searchSettingPairs(SearchSettings{ExactSearch: true, HNSWEfSearch: 100, HNSWIterativeScan: IterativeRelaxedOrder, HNSWMaxScanTuples: 5000})
	want := []settingPair{
		{"enable_indexscan", "off"},
		{"enable_bitmapscan", "off"},
		{"hnsw.ef_search", "100"},
		{"hnsw.iterative_scan", "relaxed_order"},
		{"hnsw.max_scan_tuples", "5000"},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("pairs = %v, want %v", got, want)
	}
	got = searchSettingPairs(SearchSettings{IVFFlatProbes: 4, IVFFlatIterativeScan: IterativeRelaxedOrder, IVFFlatMaxProbes: 8})
	want = []settingPair{{"ivfflat.probes", "4"}, {"ivfflat.iterative_scan", "relaxed_order"}, {"ivfflat.max_probes", "8"}}
	if !slices.Equal(got, want) {
		t.Fatalf("ivfflat pairs = %v, want %v", got, want)
	}
	if got := searchSettingPairs(SearchSettings{}); len(got) != 0 {
		t.Fatalf("zero settings produced %v", got)
	}
}

func TestPgvectorAtLeast08(t *testing.T) {
	for v, want := range map[string]bool{"0.7.4": false, "0.7.99": false, "0.8.0": true, "0.8.7": true, "0.10.0": true, "1.0.0": true, " 0.8.1 ": true, "0.8": true} {
		got, err := pgvectorAtLeast08(v)
		if err != nil || got != want {
			t.Errorf("%q: got %v, %v, want %v", v, got, err, want)
		}
	}
	for _, v := range []string{"", "x", "0.x.1", "8"} {
		if _, err := pgvectorAtLeast08(v); err == nil {
			t.Errorf("%q: want an error", v)
		}
	}
}
