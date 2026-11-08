// SPDX-License-Identifier: Apache-2.0

package vector

import (
	"context"
	"math"
	"strings"
	"testing"
)

func TestRecall(t *testing.T) {
	exact := [][]int64{{1, 2, 3, 4}, {5, 6}}
	cases := []struct {
		name   string
		approx [][]int64
		want   float64
	}{
		{"all found", [][]int64{{4, 3, 2, 1}, {6, 5}}, 1},
		{"half found", [][]int64{{1, 2, 9, 9}, {5, 9}}, 0.5},
		{"none found", [][]int64{{9}, {9}}, 0},
	}
	for _, c := range cases {
		if got := Recall(exact, c.approx); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("%s: recall = %v, want %v", c.name, got, c.want)
		}
	}
	if got := Recall(nil, nil); got != 1 {
		t.Errorf("recall of no queries = %v, want 1", got)
	}
}

func TestGenerateIsDeterministicAndNormalized(t *testing.T) {
	a := Generate(20, 3, 8, 7)
	b := Generate(20, 3, 8, 7)
	for i := range a.Vectors {
		for j := range a.Vectors[i] {
			if a.Vectors[i][j] != b.Vectors[i][j] {
				t.Fatalf("vector %d differs between two runs with the same seed", i)
			}
		}
		var sum float64
		for _, x := range a.Vectors[i] {
			sum += float64(x) * float64(x)
		}
		if math.Abs(sum-1) > 1e-4 {
			t.Fatalf("vector %d has squared norm %v, want 1", i, sum)
		}
	}
	if len(a.Queries) != 3 || a.Dim != 8 {
		t.Fatalf("dataset shape wrong: %d queries, dim %d", len(a.Queries), a.Dim)
	}
	if got := VectorText([]float32{1, 0.5}); got != "[1,0.5]" {
		t.Fatalf("VectorText = %q", got)
	}
}

// fakeDB returns a fixed approximate result per ef_search so Sweep's choice is
// predictable without a database.
type fakeDB struct {
	ef    int
	exact bool
	hits  map[int]int // ef -> how many of the 2 exact ids the approximate result keeps
}

func (f *fakeDB) TopK(_ context.Context, _ []float32, _ int) ([]int64, error) {
	if f.exact {
		return []int64{1, 2}, nil
	}
	out := []int64{1, 2}
	for i := f.hits[f.ef]; i < 2; i++ {
		out[i] = int64(100 + i)
	}
	return out, nil
}

func (f *fakeDB) WithSearchEf(_ context.Context, ef int, fn func() error) error {
	f.ef = ef
	return fn()
}

func (f *fakeDB) WithExact(_ context.Context, fn func() error) error {
	f.exact = true
	defer func() { f.exact = false }()
	return fn()
}

func TestSweepPicksSmallestEfReachingTarget(t *testing.T) {
	ds := Generate(1, 4, 2, 1)
	db := &fakeDB{hits: map[int]int{10: 0, 20: 1, 40: 2, 80: 2}}
	ef, recall, err := Sweep(context.Background(), db, ds, 2, 1.0, []int{10, 20, 40, 80})
	if err != nil {
		t.Fatal(err)
	}
	if ef != 40 || recall != 1 {
		t.Fatalf("Sweep = ef %d recall %v, want ef 40 recall 1", ef, recall)
	}
	ef, recall, err = Sweep(context.Background(), db, ds, 2, 1.1, []int{10, 20})
	if err != nil {
		t.Fatal(err)
	}
	if ef != 20 || recall != 0.5 {
		t.Fatalf("unreachable target: Sweep = ef %d recall %v, want the largest ef 20 and its recall 0.5", ef, recall)
	}
	if _, _, err := Sweep(context.Background(), db, ds, 2, 0.9, nil); err == nil || !strings.Contains(err.Error(), "at least one") {
		t.Fatalf("empty ef list error = %v", err)
	}
}
