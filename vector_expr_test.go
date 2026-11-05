// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"errors"
	"math"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/vector"
)

func renderErr(d *dialect.Dialect, e Expression[float64]) (string, error) {
	c, err := compile(d, e.AppendSQL)
	return c.SQL, err
}

func renderForTest(t *testing.T, d *dialect.Dialect, e Expression[float64]) string {
	t.Helper()
	s, err := renderErr(d, e)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func renderArgs(t *testing.T, d *dialect.Dialect, e Expression[float64]) []any {
	t.Helper()
	c, err := compile(d, e.AppendSQL)
	if err != nil {
		t.Fatal(err)
	}
	return c.Args
}

func TestVectorDistanceSQL(t *testing.T) {
	pg := dialect.For(dialect.Postgres)
	col := NewVectorColumn[vector.Vector]("docs", "emb", 3, 0)
	q := vector.Vector{4, 5, 6}
	if got := renderForTest(t, pg, L2Distance(col, q)); got != `("docs"."emb" <-> $1)` {
		t.Fatalf("L2 SQL = %s", got)
	}
	if args := renderArgs(t, pg, L2Distance(col, q)); args[0].(vector.Vector)[0] != 4 {
		t.Fatalf("bound arg = %v", args[0])
	}
	if got := renderForTest(t, pg, InnerProductDistance(col, q)); got != `("docs"."emb" <#> $1)` {
		t.Fatalf("IP SQL = %s", got)
	}
	if got := renderForTest(t, pg, CosineDistance(col, q)); got != `("docs"."emb" <=> $1)` {
		t.Fatalf("cosine SQL = %s", got)
	}
	if got := renderForTest(t, pg, L1Distance(col, q)); got != `("docs"."emb" <+> $1)` {
		t.Fatalf("L1 SQL = %s", got)
	}
	sq := dialect.For(dialect.SQLite)
	if got := renderForTest(t, sq, CosineDistance(col, q)); got != `vec_distance_cosine("docs"."emb", ?)` {
		t.Fatalf("sqlite cosine SQL = %s", got)
	}
	if got := renderForTest(t, sq, L1Distance(col, q)); got != `vec_distance_l1("docs"."emb", ?)` {
		t.Fatalf("sqlite L1 SQL = %s", got)
	}
	// Reference values computed in Go without SQL.
	a := []float64{1, 2, 3}
	b := []float64{4, 5, 6}
	var dot, na, nb, l1, sumSq float64
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
		d := a[i] - b[i]
		l1 += math.Abs(d)
		sumSq += d * d
	}
	refL2, refCos, refIP := math.Sqrt(sumSq), 1-dot/(math.Sqrt(na)*math.Sqrt(nb)), -dot
	if math.Abs(refL2-math.Sqrt(27)) > 1e-5 || math.Abs(l1-9) > 1e-5 || math.Abs(refIP+32) > 1e-5 || math.Abs(refCos-0.0253682) > 1e-5 {
		t.Fatalf("reference values: l2=%v l1=%v ip=%v cos=%v", refL2, l1, refIP, refCos)
	}
}

func TestVectorDistanceDimensionMismatch(t *testing.T) {
	col := NewVectorColumn[vector.Vector]("docs", "emb", 3, 0)
	_, err := renderErr(dialect.For(dialect.Postgres), L2Distance(col, vector.Vector{1, 2}))
	if !errors.Is(err, vector.ErrInvalidVector) {
		t.Fatalf("err = %v, want ErrInvalidVector", err)
	}
}

func TestSQLiteVectorDistanceSQL(t *testing.T) {
	sq := dialect.For(dialect.SQLite)
	bits := NewVectorColumn[vector.Bits]("docs", "bits", 8, 0)
	q := vector.Bits{Len: 8, Data: []byte{0xB0}}
	if got := renderForTest(t, sq, HammingDistance(bits, q)); got != `vec_distance_hamming(vec_bit("docs"."bits"), vec_bit(?))` {
		t.Fatalf("sqlite hamming SQL = %s", got)
	}
	if args := renderArgs(t, sq, HammingDistance(bits, q)); len(args) != 1 {
		t.Fatalf("sqlite hamming args = %v, want one bit blob", args)
	}
	col := NewVectorColumn[vector.Vector]("docs", "emb", 3, 0)
	if _, err := renderErr(sq, InnerProductDistance(col, vector.Vector{1, 2, 3})); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("sqlite ip: err = %v, want ErrUnsupported", err)
	}
	if _, err := renderErr(sq, JaccardDistance(bits, q)); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("sqlite jaccard: err = %v, want ErrUnsupported", err)
	}
}

// TestVectorDistanceMatrix covers every metric x type x dialect cell: the
// rendered operator or function, or the build-time error.
func TestVectorDistanceMatrix(t *testing.T) {
	pg, sq := dialect.For(dialect.Postgres), dialect.For(dialect.SQLite)
	dense := func(m dialect.Metric) map[string]Expression[float64] {
		v := NewVectorColumn[vector.Vector]("t", "c", 2, 0)
		h := NewVectorColumn[vector.HalfVector]("t", "c", 2, 0)
		s := NewVectorColumn[vector.SparseVector]("t", "c", 2, 0)
		sv := vector.SparseVector{Dim: 2, Indices: []int32{0}, Values: []float32{1}}
		switch m {
		case dialect.MetricL2:
			return map[string]Expression[float64]{"vector": L2Distance(v, vector.Vector{1, 2}), "halfvec": L2Distance(h, vector.HalfVector{1, 2}), "sparsevec": L2Distance(s, sv)}
		case dialect.MetricInnerProduct:
			return map[string]Expression[float64]{"vector": InnerProductDistance(v, vector.Vector{1, 2}), "halfvec": InnerProductDistance(h, vector.HalfVector{1, 2}), "sparsevec": InnerProductDistance(s, sv)}
		case dialect.MetricCosine:
			return map[string]Expression[float64]{"vector": CosineDistance(v, vector.Vector{1, 2}), "halfvec": CosineDistance(h, vector.HalfVector{1, 2}), "sparsevec": CosineDistance(s, sv)}
		default:
			return map[string]Expression[float64]{"vector": L1Distance(v, vector.Vector{1, 2}), "halfvec": L1Distance(h, vector.HalfVector{1, 2}), "sparsevec": L1Distance(s, sv)}
		}
	}
	ops := map[dialect.Metric]string{dialect.MetricL2: "<->", dialect.MetricInnerProduct: "<#>", dialect.MetricCosine: "<=>", dialect.MetricL1: "<+>"}
	for m, op := range ops {
		for typ, e := range dense(m) {
			got, err := renderErr(pg, e)
			if want := `("t"."c" ` + op + ` $1)`; err != nil || got != want {
				t.Errorf("pg %s %s: %q, %v; want %q", m, typ, got, err, want)
			}
			_, err = renderErr(sq, e)
			sqliteOK := typ == "vector" && m != dialect.MetricInnerProduct
			if sqliteOK && err != nil {
				t.Errorf("sqlite %s %s: %v, want ok", m, typ, err)
			}
			if !sqliteOK && !errors.Is(err, ErrUnsupported) {
				t.Errorf("sqlite %s %s: err = %v, want ErrUnsupported", m, typ, err)
			}
		}
	}
	bc := NewVectorColumn[vector.Bits]("t", "c", 8, 0)
	bq := vector.Bits{Len: 8, Data: []byte{1}}
	if got := renderForTest(t, pg, HammingDistance(bc, bq)); got != `("t"."c" <~> $1)` {
		t.Errorf("pg hamming = %s", got)
	}
	if got := renderForTest(t, pg, JaccardDistance(bc, bq)); got != `("t"."c" <%> $1)` {
		t.Errorf("pg jaccard = %s", got)
	}
	// Arguments are bound, never inlined.
	if c, _ := compile(pg, L2Distance(NewVectorColumn[vector.Vector]("t", "c", 2, 0), vector.Vector{7, 8}).AppendSQL); len(c.Args) != 1 {
		t.Errorf("args = %v, want one bound vector", c.Args)
	}
	// MySQL has no vector support at all.
	my := dialect.MySQLFlavor(dialect.FlavorMySQL)
	if _, err := renderErr(my, L2Distance(NewVectorColumn[vector.Vector]("t", "c", 2, 0), vector.Vector{1, 2})); !errors.Is(err, ErrUnsupported) {
		t.Errorf("mysql: err = %v, want ErrUnsupported", err)
	}
	// Invalid values fail at build time (NaN).
	if _, err := renderErr(pg, L2Distance(NewVectorColumn[vector.Vector]("t", "c", 1, 0), vector.Vector{float32(math.NaN())})); !errors.Is(err, vector.ErrInvalidVector) {
		t.Errorf("NaN: err = %v, want ErrInvalidVector", err)
	}
}

// TestVectorDistanceInQueries renders distances in SELECT, WHERE and ORDER BY.
func TestVectorDistanceInQueries(t *testing.T) {
	pg := dialect.For(dialect.Postgres)
	col := NewVectorColumn[vector.Vector]("docs", "emb", 3, 0)
	d := L2Distance(col, vector.Vector{1, 2, 3})
	radius := NewColumn[float64]("docs", "radius", 0)
	got, err := compile(pg, func(b *Builder) {
		b.WriteString("SELECT ")
		d.AppendSQL(b)
		b.WriteString(" WHERE ")
		radius.GtExpr(d).AppendSQL(b)
		b.WriteString(" ORDER BY ")
		d.order().appendSQL(b)
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `SELECT ("docs"."emb" <-> $1) WHERE "docs"."radius" > ("docs"."emb" <-> $2) ORDER BY ("docs"."emb" <-> $3) ASC`
	if got.SQL != want || len(got.Args) != 3 {
		t.Fatalf("SQL = %s args=%d", got.SQL, len(got.Args))
	}
	desc := d.Desc()
	if !desc.Desc {
		t.Fatal("Desc() must order descending")
	}
}

func TestVectorColumnBasics(t *testing.T) {
	pg := dialect.For(dialect.Postgres)
	col := NewVectorColumn[vector.Vector]("docs", "emb", 3, 0)
	if col.ColumnName() != "emb" || col.Dim() != 3 {
		t.Fatalf("name/dim = %s/%d", col.ColumnName(), col.Dim())
	}
	c, err := compile(pg, col.As("d").IsNull().AppendSQL)
	if err != nil || c.SQL != `"d"."emb" IS NULL` {
		t.Fatalf("IsNull = %q, %v", c.SQL, err)
	}
	if a := col.Set(vector.Vector{1, 2, 3}); a.ColumnName() != "emb" {
		t.Fatalf("Set column = %s", a.ColumnName())
	}
}
