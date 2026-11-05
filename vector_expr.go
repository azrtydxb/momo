// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"fmt"
	"slices"

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

// DistanceExpr is a vector distance between a column and a bound query
// vector. It is an Expression[float64], so it works in the select list, in
// comparisons against float columns and in ORDER BY, where it orders
// ascending (nearest first) unless Desc is used.
type DistanceExpr struct {
	table, name string
	dim         int
	metric      dialect.Metric
	arg         any
}

func newDistance[V VectorValue](c VectorColumn[V], m dialect.Metric, v V) DistanceExpr {
	return DistanceExpr{table: c.table, name: c.name, dim: c.dim, metric: m, arg: v}
}

// L2Distance is the Euclidean distance (pgvector <->).
func L2Distance[V denseVector](c VectorColumn[V], v V) DistanceExpr {
	return newDistance(c, dialect.MetricL2, v)
}

// InnerProductDistance is the negative inner product (pgvector <#>).
func InnerProductDistance[V denseVector](c VectorColumn[V], v V) DistanceExpr {
	return newDistance(c, dialect.MetricInnerProduct, v)
}

// CosineDistance is the cosine distance (pgvector <=>).
func CosineDistance[V denseVector](c VectorColumn[V], v V) DistanceExpr {
	return newDistance(c, dialect.MetricCosine, v)
}

// L1Distance is the taxicab distance (pgvector <+>).
func L1Distance[V denseVector](c VectorColumn[V], v V) DistanceExpr {
	return newDistance(c, dialect.MetricL1, v)
}

// HammingDistance is the Hamming distance of bit vectors (pgvector <~>).
func HammingDistance[V bitVector](c VectorColumn[V], v V) DistanceExpr {
	return newDistance(c, dialect.MetricHamming, v)
}

// JaccardDistance is the Jaccard distance of bit vectors (pgvector <%>).
func JaccardDistance[V bitVector](c VectorColumn[V], v V) DistanceExpr {
	return newDistance(c, dialect.MetricJaccard, v)
}

func (DistanceExpr) resultType() float64 { return 0 }

// Asc orders nearest first.
func (e DistanceExpr) Asc() Order { return Order{Expr: e} }

// Desc orders farthest first.
func (e DistanceExpr) Desc() Order { return Order{Expr: e, Desc: true} }

func (e DistanceExpr) order() Order { return e.Asc() }

// validateValue checks v against dim through its Validate method.
func validateValue(v any, dim int) error {
	if vv, ok := v.(interface{ Validate(int) error }); ok {
		return vv.Validate(dim)
	}
	return nil
}

// AppendSQL renders the distance, or records a build error when the dialect
// lacks the metric or the argument does not fit the column.
func (e DistanceExpr) AppendSQL(b *Builder) {
	unavailable := func() {
		b.SetErr(&Error{Kind: ErrUnsupported, Op: "build", Table: e.table, Column: e.name,
			Err: fmt.Errorf("%s distance not available on %s", e.metric, b.d.Name)})
	}
	if !slices.Contains(b.d.Vector.Metrics, e.metric) {
		unavailable()
		return
	}
	if err := validateValue(e.arg, e.dim); err != nil {
		b.SetErr(&Error{Kind: ErrValidation, Op: "build", Table: e.table, Column: e.name, Err: err})
		return
	}
	if b.d.Name == dialect.Postgres {
		b.WriteString("(")
		b.QualifiedIdent(e.table, e.name)
		b.WriteString(" " + pgOperators[e.metric] + " ")
		b.Arg(e.arg)
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
	b.Arg(e.arg)
	if bitOperands {
		b.WriteString(")")
	}
	b.WriteString(")")
}
