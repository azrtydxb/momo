// SPDX-License-Identifier: Apache-2.0

// Package vecop holds the vector distance builders: L2, InnerProduct, Cosine,
// L1, Hamming and Jaccard. Each takes a vector column (momo.VectorColumn or
// one of its variants) and a query vector and returns an Expr, a
// momo.Expression[float64] that works in the select list, in comparisons
// against float columns and in ORDER BY, where it orders nearest first unless
// Desc is used.
//
// PostgreSQL renders the pgvector operators (<->, <#>, <=>, <+>, <~>, <%>),
// SQLite renders the sqlite-vec functions and MariaDB renders VEC_DISTANCE_*.
// A metric the dialect lacks, or a vector type it cannot store, fails the
// build with momo.ErrUnsupported. The metric functions are generic over the
// vector type, so a bit vector column cannot be given to a dense-vector
// metric and the compiler says so.
//
// The package builds on the exported extension API of the root package (Ops,
// VectorColumnOf, Builder.ArgFor and Builder.MarkSensitive). The vector column
// types themselves stay in package momo because generated code names them.
package vecop

import (
	"fmt"
	"slices"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/vector"
)

type denseVector interface {
	vector.Vector | vector.HalfVector | vector.SparseVector
}

type bitVector interface{ vector.Bits }

// pgOperators maps each metric to its pgvector operator.
var pgOperators = map[dialect.Metric]string{
	dialect.MetricL2: "<->", dialect.MetricInnerProduct: "<#>", dialect.MetricCosine: "<=>",
	dialect.MetricL1: "<+>", dialect.MetricHamming: "<~>", dialect.MetricJaccard: "<%>",
}

// sqliteFunctions maps each metric to its sqlite-vec scalar function.
var sqliteFunctions = map[dialect.Metric]string{
	dialect.MetricL2:      "vec_distance_l2",
	dialect.MetricL1:      "vec_distance_l1",
	dialect.MetricCosine:  "vec_distance_cosine",
	dialect.MetricHamming: "vec_distance_hamming",
}

// mariaDBFunctions maps each metric MariaDB 11.8 provides to its distance
// function. MySQL (any version) has no distance function outside HeatWave, so
// its dialect lists no metric and a distance never reaches this table.
var mariaDBFunctions = map[dialect.Metric]string{
	dialect.MetricL2:     "VEC_DISTANCE_EUCLIDEAN",
	dialect.MetricCosine: "VEC_DISTANCE_COSINE",
}

// Expr is a vector distance between a column and a bound query vector. It is
// a momo.Expression[float64] with the comparison and ordering methods of any
// scalar expression: Asc orders nearest first and Desc farthest first.
type Expr struct {
	momo.Ops[float64]
	table, name string
	dim         int
	metric      dialect.Metric
	arg         any
	flags       uint16
}

func newExpr[V momo.VectorValue](src momo.VectorSource[V], m dialect.Metric, v V) Expr {
	c := momo.VectorColumnOf(src)
	e := Expr{table: c.Table(), name: c.ColumnName(), dim: c.Dim(), metric: m, arg: v, flags: c.Flags()}
	e.Ops = momo.NewOps[float64](e)
	return e
}

// L2 is the Euclidean distance (pgvector <->).
func L2[V denseVector, C momo.VectorSource[V]](c C, v V) Expr {
	return newExpr(c, dialect.MetricL2, v)
}

// InnerProduct is the negative inner product (pgvector <#>).
func InnerProduct[V denseVector, C momo.VectorSource[V]](c C, v V) Expr {
	return newExpr(c, dialect.MetricInnerProduct, v)
}

// Cosine is the cosine distance (pgvector <=>).
func Cosine[V denseVector, C momo.VectorSource[V]](c C, v V) Expr {
	return newExpr(c, dialect.MetricCosine, v)
}

// L1 is the taxicab distance (pgvector <+>).
func L1[V denseVector, C momo.VectorSource[V]](c C, v V) Expr {
	return newExpr(c, dialect.MetricL1, v)
}

// Hamming is the Hamming distance of bit vectors (pgvector <~>).
func Hamming[V bitVector, C momo.VectorSource[V]](c C, v V) Expr {
	return newExpr(c, dialect.MetricHamming, v)
}

// Jaccard is the Jaccard distance of bit vectors (pgvector <%>).
func Jaccard[V bitVector, C momo.VectorSource[V]](c C, v V) Expr {
	return newExpr(c, dialect.MetricJaccard, v)
}

// validateValue checks v against dim through its Validate method. A dim of
// zero marks a dimensionless column (declared without the dim tag): the
// column accepts any dimension, so v is checked against its own, which
// still rejects NaN, Inf and malformed values. pgvector itself rejects a
// distance between two vectors of different sizes, so the query must
// filter the rows to one size first.
func validateValue(v any, dim int) error {
	if dim == 0 {
		dim = ownDim(v)
	}
	if vv, ok := v.(interface{ Validate(int) error }); ok {
		return vv.Validate(dim)
	}
	return nil
}

// ownDim is the dimension v carries itself.
func ownDim(v any) int {
	switch x := v.(type) {
	case vector.Vector:
		return len(x)
	case vector.HalfVector:
		return len(x)
	case vector.SparseVector:
		return x.Dim
	case vector.Bits:
		return x.Len
	}
	return 0
}

// AppendSQL renders the distance, or records a build error when the dialect
// lacks the metric or the argument does not fit the column. Other dialects
// fail the build with momo.ErrUnsupported.
func (e Expr) AppendSQL(b *momo.Builder) {
	d := b.Dialect()
	unavailable := func() {
		b.SetErr(&momo.Error{Kind: momo.ErrUnsupported, Op: "build", Table: e.table, Column: e.name,
			Err: fmt.Errorf("%s distance not available on %s", e.metric, d.Name)})
	}
	if !slices.Contains(d.Vector.Metrics, e.metric) {
		unavailable()
		return
	}
	if err := validateValue(e.arg, e.dim); err != nil {
		b.SetErr(&momo.Error{Kind: momo.ErrValidation, Op: "build", Table: e.table, Column: e.name, Err: err})
		return
	}
	if e.flags&momo.FlagSensitive != 0 {
		b.MarkSensitive() // an enclosing predicate masks its other operands too
	}
	if d.Name == dialect.Postgres {
		b.WriteString("(")
		b.QualifiedIdent(e.table, e.name)
		b.WriteString(" " + pgOperators[e.metric] + " ")
		b.ArgFor(e.flags, e.arg)
		b.WriteString(")")
		return
	}
	if d.Name == dialect.MySQL {
		fn, ok := mariaDBFunctions[e.metric]
		if !ok {
			unavailable()
			return
		}
		b.WriteString(fn + "(")
		b.QualifiedIdent(e.table, e.name)
		b.WriteString(", ")
		b.ArgFor(e.flags, e.arg) // wrapped in VEC_FromText
		b.WriteString(")")
		return
	}
	fn, ok := sqliteFunctions[e.metric]
	if !ok {
		unavailable()
		return
	}
	bitOperands := e.metric == dialect.MetricHamming
	b.WriteString(fn + "(")
	if bitOperands {
		b.WriteString("vec_bit(")
	}
	b.QualifiedIdent(e.table, e.name)
	if bitOperands {
		b.WriteString(")")
	}
	b.WriteString(", ")
	if bitOperands {
		b.WriteString("vec_bit(")
	}
	b.ArgFor(e.flags, e.arg)
	if bitOperands {
		b.WriteString(")")
	}
	b.WriteString(")")
}
