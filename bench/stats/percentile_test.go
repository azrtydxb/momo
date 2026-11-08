// SPDX-License-Identifier: Apache-2.0

package stats

import (
	"testing"
	"time"
)

func TestPercentiles(t *testing.T) {
	var s []time.Duration
	for i := 100; i >= 1; i-- {
		s = append(s, time.Duration(i))
	}
	cases := map[float64]time.Duration{50: 50, 95: 95, 99: 99, 100: 100}
	for p, want := range cases {
		if got := Percentile(s, p); got != want {
			t.Errorf("p%v = %v, want %v", p, got, want)
		}
	}
	if len(s) != 100 || s[0] != 100 {
		t.Fatal("Percentile must not reorder the caller's slice")
	}
}

func TestPercentileEdges(t *testing.T) {
	if got := Percentile(nil, 50); got != 0 {
		t.Errorf("empty samples: p50 = %v, want 0", got)
	}
	one := []time.Duration{7}
	for _, p := range []float64{1, 50, 99, 100} {
		if got := Percentile(one, p); got != 7 {
			t.Errorf("single sample: p%v = %v, want 7", p, got)
		}
	}
	if got := Percentile([]time.Duration{1, 2, 3, 4}, 50); got != 2 {
		t.Errorf("nearest rank p50 of 4 samples = %v, want 2", got)
	}
}
