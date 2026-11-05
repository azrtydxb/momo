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
	}
	for _, s := range bad {
		if err := s.Validate(); !errors.Is(err, ErrValidation) {
			t.Errorf("%+v: err = %v, want ErrValidation", s, err)
		}
	}
	good := []SearchSettings{
		{},
		{HNSWEfSearch: 1000, HNSWIterativeScan: IterativeStrictOrder},
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
