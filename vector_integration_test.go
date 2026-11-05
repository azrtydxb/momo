// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"math"
	"slices"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/vector"
)

// vecDoc is hand-written: SQL() needs only the table name and the columns.
type vecDoc struct {
	ID  int64
	Emb vector.Vector
}

var vecDocMeta = momo.Meta[vecDoc]{
	Table:      "docs",
	Columns:    []momo.ColumnMeta{{Name: "id"}, {Name: "emb"}},
	Ptr:        func(m *vecDoc, i int) any { return []any{&m.ID, &m.Emb}[i] },
	Value:      func(m *vecDoc, i int) any { return []any{m.ID, m.Emb}[i] },
	Identity:   -1,
	Version:    -1,
	SoftDelete: -1,
	CreatedAt:  -1,
	UpdatedAt:  -1,
	PK:         []int{0},
}

func (d *vecDoc) MomoMeta() *momo.Meta[vecDoc] { return &vecDocMeta }

func TestVectorDistances(t *testing.T) {
	db := testopen.Postgres(t)
	mustExecPG(t, db, "CREATE TABLE docs (id bigint PRIMARY KEY, emb vector(3))")
	rows := []vector.Vector{{1, 2, 3}, {4, 5, 6}, {10, 10, 10}}
	for i, v := range rows {
		mustExecPG(t, db, "INSERT INTO docs (id, emb) VALUES (?, ?)", int64(i+1), v)
	}
	col := momo.NewVectorColumn[vector.Vector]("docs", "emb", 3, 0)
	c, err := momo.Select[vecDoc, *vecDoc](db).OrderBy(momo.L2Distance(col, vector.Vector{4, 5, 6}).Asc()).Limit(3).SQL()
	if err != nil {
		t.Fatal(err)
	}
	// Reference distances to {4,5,6}: id 2 is 0, id 1 is sqrt(27), id 3 is sqrt(75).
	if got, want := queryIDs(t, pgPool(t), c), []int64{2, 1, 3}; !slices.Equal(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}

	// Reference math in plain Go. Inner product is pgvector's `<#>`, which
	// returns the NEGATIVE dot product so that ascending order means "closest".
	q := vector.Vector{4, 5, 6}
	l2 := func(a, b vector.Vector) float64 {
		var sum float64
		for i := range a {
			d := float64(a[i]) - float64(b[i])
			sum += d * d
		}
		return math.Sqrt(sum)
	}
	l1 := func(a, b vector.Vector) float64 {
		var sum float64
		for i := range a {
			sum += math.Abs(float64(a[i]) - float64(b[i]))
		}
		return sum
	}
	negDot := func(a, b vector.Vector) float64 {
		var dot float64
		for i := range a {
			dot += float64(a[i]) * float64(b[i])
		}
		return -dot
	}
	cosine := func(a, b vector.Vector) float64 {
		var dot, na, nb float64
		for i := range a {
			dot += float64(a[i]) * float64(b[i])
			na += float64(a[i]) * float64(a[i])
			nb += float64(b[i]) * float64(b[i])
		}
		return 1 - dot/(math.Sqrt(na)*math.Sqrt(nb))
	}
	idCol := momo.ColumnOf[int64]("docs", "id")
	cases := []struct {
		name string
		expr momo.Expression[float64]
		ref  func(a, b vector.Vector) float64
	}{
		{"L2Distance", momo.L2Distance(col, q), l2},
		{"InnerProductDistance", momo.InnerProductDistance(col, q), negDot},
		{"CosineDistance", momo.CosineDistance(col, q), cosine},
		{"L1Distance", momo.L1Distance(col, q), l1},
	}
	for _, tc := range cases {
		// The select list must match the two mapped fields of vecDoc, so it is id plus the distance.
		c, err := momo.Select[vecDoc, *vecDoc](db).Columns(idCol, tc.expr).SQL()
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		got := queryIDFloat(t, pgPool(t), c)
		if len(got) != len(rows) {
			t.Fatalf("%s: got %d rows, want %d", tc.name, len(got), len(rows))
		}
		for i, v := range rows {
			id, want := int64(i+1), tc.ref(v, q)
			if g, ok := got[id]; !ok || math.Abs(g-want) > 1e-5 {
				t.Errorf("%s id %d = %v, want %v (1e-5)", tc.name, id, g, want)
			}
		}
	}
}
