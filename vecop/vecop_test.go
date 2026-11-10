// SPDX-License-Identifier: Apache-2.0

package vecop_test

import (
	"errors"
	"math"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/vecop"
	"github.com/azrtydxb/momo/vector"
)

func renderErr(d *dialect.Dialect, e momo.Expression[float64]) (string, []any, error) {
	b := momo.NewBuilder(d)
	e.AppendSQL(b)
	return b.String(), b.Args(), b.Err()
}

func renderForTest(t *testing.T, d *dialect.Dialect, e momo.Expression[float64]) string {
	t.Helper()
	s, _, err := renderErr(d, e)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func renderArgs(t *testing.T, d *dialect.Dialect, e momo.Expression[float64]) []any {
	t.Helper()
	_, args, err := renderErr(d, e)
	if err != nil {
		t.Fatal(err)
	}
	return args
}

func TestDistanceSQL(t *testing.T) {
	pg := dialect.For(dialect.Postgres)
	col := momo.NewVectorColumn[vector.Vector]("docs", "emb", 3, 0)
	q := vector.Vector{4, 5, 6}
	if got := renderForTest(t, pg, vecop.L2(col, q)); got != `("docs"."emb" <-> $1)` {
		t.Fatalf("L2 SQL = %s", got)
	}
	if args := renderArgs(t, pg, vecop.L2(col, q)); args[0].(vector.Vector)[0] != 4 {
		t.Fatalf("bound arg = %v", args[0])
	}
	if got := renderForTest(t, pg, vecop.InnerProduct(col, q)); got != `("docs"."emb" <#> $1)` {
		t.Fatalf("IP SQL = %s", got)
	}
	if got := renderForTest(t, pg, vecop.Cosine(col, q)); got != `("docs"."emb" <=> $1)` {
		t.Fatalf("cosine SQL = %s", got)
	}
	if got := renderForTest(t, pg, vecop.L1(col, q)); got != `("docs"."emb" <+> $1)` {
		t.Fatalf("L1 SQL = %s", got)
	}
	sq := dialect.For(dialect.SQLite)
	if got := renderForTest(t, sq, vecop.Cosine(col, q)); got != `vec_distance_cosine("docs"."emb", ?)` {
		t.Fatalf("sqlite cosine SQL = %s", got)
	}
	if got := renderForTest(t, sq, vecop.L1(col, q)); got != `vec_distance_l1("docs"."emb", ?)` {
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

func TestDistanceDimensionMismatch(t *testing.T) {
	col := momo.NewVectorColumn[vector.Vector]("docs", "emb", 3, 0)
	_, _, err := renderErr(dialect.For(dialect.Postgres), vecop.L2(col, vector.Vector{1, 2}))
	if !errors.Is(err, vector.ErrInvalidVector) {
		t.Fatalf("err = %v, want ErrInvalidVector", err)
	}
	if !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("err = %v, want ErrValidation", err)
	}
}

// TestDistanceDimensionless: a column declared without dim has Dim 0 and
// takes a query vector of any size; NaN and an empty vector are still
// refused.
func TestDistanceDimensionless(t *testing.T) {
	pg := dialect.For(dialect.Postgres)
	col := momo.NewVectorColumn[vector.Vector]("docs", "emb", 0, 0)
	for _, q := range []vector.Vector{{1, 2}, {1, 2, 3, 4, 5}} {
		got, _, err := renderErr(pg, vecop.Cosine(col, q))
		if want := `("docs"."emb" <=> $1)`; err != nil || got != want {
			t.Errorf("len %d: %q, %v; want %q", len(q), got, err, want)
		}
	}
	if _, _, err := renderErr(pg, vecop.Cosine(col, vector.Vector{1, float32(math.NaN())})); !errors.Is(err, vector.ErrInvalidVector) {
		t.Errorf("NaN: err = %v, want ErrInvalidVector", err)
	}
	if _, _, err := renderErr(pg, vecop.Cosine(col, vector.Vector{})); !errors.Is(err, vector.ErrInvalidVector) {
		t.Errorf("empty: err = %v, want ErrInvalidVector", err)
	}
	h := momo.NewVectorColumn[vector.HalfVector]("docs", "h", 0, 0)
	if _, _, err := renderErr(pg, vecop.L2(h, vector.HalfVector{1, 2, 3})); err != nil {
		t.Errorf("halfvec: %v", err)
	}
}

func TestSQLiteDistanceSQL(t *testing.T) {
	sq := dialect.For(dialect.SQLite)
	bits := momo.NewVectorColumn[vector.Bits]("docs", "bits", 8, 0)
	q := vector.Bits{Len: 8, Data: []byte{0xB0}}
	if got := renderForTest(t, sq, vecop.Hamming(bits, q)); got != `vec_distance_hamming(vec_bit("docs"."bits"), vec_bit(?))` {
		t.Fatalf("sqlite hamming SQL = %s", got)
	}
	if args := renderArgs(t, sq, vecop.Hamming(bits, q)); len(args) != 1 {
		t.Fatalf("sqlite hamming args = %v, want one bit blob", args)
	}
	col := momo.NewVectorColumn[vector.Vector]("docs", "emb", 3, 0)
	if _, _, err := renderErr(sq, vecop.InnerProduct(col, vector.Vector{1, 2, 3})); !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("sqlite ip: err = %v, want ErrUnsupported", err)
	}
	if _, _, err := renderErr(sq, vecop.Jaccard(bits, q)); !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("sqlite jaccard: err = %v, want ErrUnsupported", err)
	}
}

// Everything sqlite-vec cannot do is ErrUnsupported when the query is built:
// the halfvec and sparsevec distances, inner product and Jaccard.
func TestSQLiteDistanceUnsupported(t *testing.T) {
	sq := dialect.For(dialect.SQLite)
	half := momo.NewVectorColumn[vector.HalfVector]("docs", "h", 2, 0)
	sparse := momo.NewVectorColumn[vector.SparseVector]("docs", "s", 2, 0)
	sv := vector.SparseVector{Dim: 2, Indices: []int32{0}, Values: []float32{1}}
	bits := momo.NewVectorColumn[vector.Bits]("docs", "b", 8, 0)
	dense := momo.NewVectorColumn[vector.Vector]("docs", "emb", 2, 0)
	for name, e := range map[string]momo.Expression[float64]{
		"halfvec l2":    vecop.L2(half, vector.HalfVector{1, 2}),
		"sparsevec l1":  vecop.L1(sparse, sv),
		"jaccard":       vecop.Jaccard(bits, vector.Bits{Len: 8, Data: []byte{1}}),
		"inner product": vecop.InnerProduct(dense, vector.Vector{1, 2}),
	} {
		if _, _, err := renderErr(sq, e); !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("%s: err = %v, want ErrUnsupported", name, err)
		}
	}
}

func TestMariaDBDistanceSQL(t *testing.T) {
	ma := dialect.MySQLFlavor(dialect.FlavorMariaDB)
	my := dialect.MySQLFlavor(dialect.FlavorMySQL)
	col := momo.NewVectorColumn[vector.Vector]("docs", "emb", 3, 0)
	q := vector.Vector{1, 2, 3}

	if got, want := renderForTest(t, ma, vecop.Cosine(col, q)), "VEC_DISTANCE_COSINE(`docs`.`emb`, VEC_FromText(?))"; got != want {
		t.Fatalf("MariaDB cosine = %s, want %s", got, want)
	}
	if got, want := renderForTest(t, ma, vecop.L2(col, q)), "VEC_DISTANCE_EUCLIDEAN(`docs`.`emb`, VEC_FromText(?))"; got != want {
		t.Fatalf("MariaDB L2 = %s, want %s", got, want)
	}
	if args := renderArgs(t, ma, vecop.L2(col, q)); len(args) != 1 || args[0] != "[1,2,3]" {
		t.Fatalf("MariaDB args = %#v, want the text form [1,2,3]", args)
	}
	for _, e := range []momo.Expression[float64]{vecop.L2(col, q), vecop.Cosine(col, q), vecop.InnerProduct(col, q)} {
		if _, _, err := renderErr(my, e); !errors.Is(err, momo.ErrUnsupported) {
			t.Fatalf("MySQL distance = %v, want ErrUnsupported", err)
		}
	}
	// MariaDB offers neither inner product nor L1: no silent substitute.
	for _, e := range []momo.Expression[float64]{vecop.InnerProduct(col, q), vecop.L1(col, q)} {
		if _, _, err := renderErr(ma, e); !errors.Is(err, momo.ErrUnsupported) {
			t.Fatalf("MariaDB unsupported metric = %v, want ErrUnsupported", err)
		}
	}
	// Half precision, sparse and bit vectors have no MariaDB type.
	hc := momo.NewVectorColumn[vector.HalfVector]("docs", "emb", 3, 0)
	if _, _, err := renderErr(ma, vecop.L2(hc, vector.HalfVector{1, 2, 3})); !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("MariaDB halfvec = %v, want ErrUnsupported", err)
	}
}

// TestDistanceMatrix covers every metric x type x dialect cell: the rendered
// operator or function, or the build-time error.
func TestDistanceMatrix(t *testing.T) {
	pg, sq := dialect.For(dialect.Postgres), dialect.For(dialect.SQLite)
	dense := func(m dialect.Metric) map[string]momo.Expression[float64] {
		v := momo.NewVectorColumn[vector.Vector]("t", "c", 2, 0)
		h := momo.NewVectorColumn[vector.HalfVector]("t", "c", 2, 0)
		s := momo.NewVectorColumn[vector.SparseVector]("t", "c", 2, 0)
		sv := vector.SparseVector{Dim: 2, Indices: []int32{0}, Values: []float32{1}}
		switch m {
		case dialect.MetricL2:
			return map[string]momo.Expression[float64]{"vector": vecop.L2(v, vector.Vector{1, 2}), "halfvec": vecop.L2(h, vector.HalfVector{1, 2}), "sparsevec": vecop.L2(s, sv)}
		case dialect.MetricInnerProduct:
			return map[string]momo.Expression[float64]{"vector": vecop.InnerProduct(v, vector.Vector{1, 2}), "halfvec": vecop.InnerProduct(h, vector.HalfVector{1, 2}), "sparsevec": vecop.InnerProduct(s, sv)}
		case dialect.MetricCosine:
			return map[string]momo.Expression[float64]{"vector": vecop.Cosine(v, vector.Vector{1, 2}), "halfvec": vecop.Cosine(h, vector.HalfVector{1, 2}), "sparsevec": vecop.Cosine(s, sv)}
		default:
			return map[string]momo.Expression[float64]{"vector": vecop.L1(v, vector.Vector{1, 2}), "halfvec": vecop.L1(h, vector.HalfVector{1, 2}), "sparsevec": vecop.L1(s, sv)}
		}
	}
	ops := map[dialect.Metric]string{dialect.MetricL2: "<->", dialect.MetricInnerProduct: "<#>", dialect.MetricCosine: "<=>", dialect.MetricL1: "<+>"}
	for m, op := range ops {
		for typ, e := range dense(m) {
			got, _, err := renderErr(pg, e)
			if want := `("t"."c" ` + op + ` $1)`; err != nil || got != want {
				t.Errorf("pg %s %s: %q, %v; want %q", m, typ, got, err, want)
			}
			_, _, err = renderErr(sq, e)
			sqliteOK := typ == "vector" && m != dialect.MetricInnerProduct
			if sqliteOK && err != nil {
				t.Errorf("sqlite %s %s: %v, want ok", m, typ, err)
			}
			if !sqliteOK && !errors.Is(err, momo.ErrUnsupported) {
				t.Errorf("sqlite %s %s: err = %v, want ErrUnsupported", m, typ, err)
			}
		}
	}
	bc := momo.NewVectorColumn[vector.Bits]("t", "c", 8, 0)
	bq := vector.Bits{Len: 8, Data: []byte{1}}
	if got := renderForTest(t, pg, vecop.Hamming(bc, bq)); got != `("t"."c" <~> $1)` {
		t.Errorf("pg hamming = %s", got)
	}
	if got := renderForTest(t, pg, vecop.Jaccard(bc, bq)); got != `("t"."c" <%> $1)` {
		t.Errorf("pg jaccard = %s", got)
	}
	// Arguments are bound, never inlined.
	if _, args, _ := renderErr(pg, vecop.L2(momo.NewVectorColumn[vector.Vector]("t", "c", 2, 0), vector.Vector{7, 8})); len(args) != 1 {
		t.Errorf("args = %v, want one bound vector", args)
	}
	// MySQL has no vector support at all.
	my := dialect.MySQLFlavor(dialect.FlavorMySQL)
	if _, _, err := renderErr(my, vecop.L2(momo.NewVectorColumn[vector.Vector]("t", "c", 2, 0), vector.Vector{1, 2})); !errors.Is(err, momo.ErrUnsupported) {
		t.Errorf("mysql: err = %v, want ErrUnsupported", err)
	}
	// Invalid values fail at build time (NaN).
	if _, _, err := renderErr(pg, vecop.L2(momo.NewVectorColumn[vector.Vector]("t", "c", 1, 0), vector.Vector{float32(math.NaN())})); !errors.Is(err, vector.ErrInvalidVector) {
		t.Errorf("NaN: err = %v, want ErrInvalidVector", err)
	}
}

// TestDistanceInQueries renders distances in SELECT, WHERE and ORDER BY.
func TestDistanceInQueries(t *testing.T) {
	col := momo.NewVectorColumn[vector.Vector]("docs", "emb", 3, 0)
	d := vecop.L2(col, vector.Vector{1, 2, 3})
	radius := momo.NewColumn[float64]("docs", "radius", 0)

	b := momo.NewBuilder(dialect.For(dialect.Postgres))
	b.WriteString("SELECT ")
	d.AppendSQL(b)
	b.WriteString(" WHERE ")
	radius.GtExpr(d).AppendSQL(b)
	b.WriteString(" ORDER BY ")
	d.Asc().AppendOrder(b)
	if err := b.Err(); err != nil {
		t.Fatal(err)
	}
	want := `SELECT ("docs"."emb" <-> $1) WHERE "docs"."radius" > ("docs"."emb" <-> $2) ORDER BY ("docs"."emb" <-> $3) ASC`
	if b.String() != want || len(b.Args()) != 3 {
		t.Fatalf("SQL = %s args=%d", b.String(), len(b.Args()))
	}
	if d.Asc().Desc {
		t.Fatal("Asc() must order ascending")
	}
	if !d.Desc().Desc {
		t.Fatal("Desc() must order descending")
	}
}

// TestDistanceThresholdFilter renders a distance compared with a bound
// threshold, the radius filter of a nearest-neighbour search. The comparison
// methods come from momo.Ops, so the distance renders as the left operand and
// both values are bound.
func TestDistanceThresholdFilter(t *testing.T) {
	col := momo.NewVectorColumn[vector.Vector]("t", "c", 2, 0)
	q := vector.Vector{1, 2}
	for name, c := range map[string]struct {
		d    *dialect.Dialect
		want string
	}{
		"postgres": {dialect.For(dialect.Postgres), `("t"."c" <-> $1) < $2`},
		"sqlite":   {dialect.For(dialect.SQLite), `vec_distance_l2("t"."c", ?) < ?`},
	} {
		b := momo.NewBuilder(c.d)
		vecop.L2(col, q).Lt(0.5).AppendSQL(b)
		if err := b.Err(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if b.String() != c.want {
			t.Errorf("%s: SQL = %s, want %s", name, b.String(), c.want)
		}
		args := b.Args()
		if len(args) != 2 || args[1] != 0.5 {
			t.Errorf("%s: args = %#v, want the query vector and 0.5", name, args)
		}
	}
}
