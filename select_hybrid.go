// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"errors"
	"fmt"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/vector"
)

// Defaults and limits of HybridSpec.
const (
	// DefaultHybridCandidates is the candidate count of each leg when
	// HybridSpec.TextCandidates or VectorCandidates is zero.
	DefaultHybridCandidates = 100
	// DefaultHybridRRFK is the rank constant used when HybridSpec.RRFK is
	// zero (Cormack, Clarke and Buettcher, 2009).
	DefaultHybridRRFK = 60

	maxHybridCandidates = 1000
	maxHybridLimit      = 1000
)

// HybridSpec describes a PostgreSQL hybrid search: a full-text leg and a
// vector leg over one model, fused by reciprocal rank fusion (RRF).
//
// Each leg picks its own candidates (the best TextCandidates rows by
// ts_rank_cd, and the nearest VectorCandidates rows by cosine distance), ranks
// them, and a row scores the sum of 1/(RRFK+rank) over the legs it appears in.
// Ties on the score are broken by primary key ascending, within each leg's
// ranking and in the final order, so the result does not depend on row or
// insertion order. The one exception is the candidate cut-off: the vector leg
// orders by distance alone (so an hnsw or ivfflat index can serve it), and when
// rows tie on distance at the cut-off, which of them become candidates is not
// defined.
//
// The vector leg takes the dense vector type only (not halfvec, sparsevec or
// bit) and skips rows with a NULL embedding and rows whose cosine distance is
// NaN (a zero vector). The text leg computes to_tsvector over the column with
// the bound TextConfig; a GIN expression index needs a constant configuration
// in its definition, and a bound regconfig parameter may not match it, so
// without a matching plan the text leg scans the table.
type HybridSpec struct {
	// Text is the full-text column of the model, a text column of the same
	// table as Vector.
	Text       Column[string]
	TextConfig string // regconfig name such as "english"; bound as a parameter
	Query      string // websearch_to_tsquery input
	Vector     VectorColumn[vector.Vector]
	// QueryVector must match the column's dimension (any dimension for a
	// dimensionless column).
	QueryVector vector.Vector
	// TextCandidates and VectorCandidates are the candidates each leg
	// contributes, 1..1000; zero means DefaultHybridCandidates (100).
	TextCandidates   int
	VectorCandidates int
	// RRFK is the RRF rank constant, at least 1; zero means DefaultHybridRRFK (60).
	RRFK int
	// Limit is the number of rows returned, 1..1000.
	Limit int
}

// HybridResult is one fused row.
type HybridResult[T any] struct {
	Row T
	// Score is the sum of 1/(RRFK+rank) over the legs the row appears in.
	Score float64
}

// HybridQuery is a prepared hybrid search; see HybridSearch.
type HybridQuery[T any] struct {
	q    Querier
	meta *Meta[T]
	spec HybridSpec
}

// HybridSearch builds the fused full-text and vector query for model T. It
// runs on PostgreSQL with pgvector only; on another dialect SQL and All
// return an error that wraps ErrUnsupported. The model needs a single-column
// primary key. Soft-deleted rows are excluded.
//
// The statement binds, in order: the text configuration, the query text, the
// text candidate count, the query vector, the vector candidate count, the RRF
// constant and the limit.
func HybridSearch[T any, PT Modeler[T]](q Querier, spec HybridSpec) HybridQuery[T] {
	return HybridQuery[T]{q: q, meta: metaOf[T, PT](), spec: spec}
}

func hybridInvalid(format string, args ...any) error {
	return &Error{Kind: ErrValidation, Op: "hybrid search", Err: fmt.Errorf(format, args...)}
}

// normalize applies the defaults and checks the spec against the model.
func (h HybridQuery[T]) normalize() (HybridSpec, error) {
	s := h.spec
	if h.q == nil {
		return s, noQuerier("select", h.meta.Table)
	}
	if h.q.Dialect() == nil || h.q.Dialect().Name != dialect.Postgres {
		return s, &Error{Kind: ErrUnsupported, Op: "hybrid search", Table: h.meta.Table, Err: errors.New("hybrid search needs PostgreSQL with pgvector")}
	}
	if h.meta.Table == "" {
		return s, hybridInvalid("the result type is a projection, not a model with a table")
	}
	if len(h.meta.PK) != 1 {
		return s, hybridInvalid("table %s needs a single-column primary key, it has %d", h.meta.Table, len(h.meta.PK))
	}
	if s.Text.ColumnName() == "" || s.Vector.ColumnName() == "" {
		return s, hybridInvalid("Text and Vector must name columns")
	}
	if s.Query == "" {
		return s, hybridInvalid("Query is empty; a search with no text is a plain vector search")
	}
	for _, c := range []interface{ Table() string }{s.Text, s.Vector} {
		if c.Table() != h.meta.Table {
			return s, hybridInvalid("column of table %q does not belong to the model table %q", c.Table(), h.meta.Table)
		}
	}
	if s.TextConfig == "" {
		return s, hybridInvalid("TextConfig is empty; name a text search configuration such as english")
	}
	if s.TextCandidates == 0 {
		s.TextCandidates = DefaultHybridCandidates
	}
	if s.VectorCandidates == 0 {
		s.VectorCandidates = DefaultHybridCandidates
	}
	if s.RRFK == 0 {
		s.RRFK = DefaultHybridRRFK
	}
	if s.TextCandidates < 1 || s.TextCandidates > maxHybridCandidates {
		return s, hybridInvalid("TextCandidates %d is outside 1..%d", s.TextCandidates, maxHybridCandidates)
	}
	if s.VectorCandidates < 1 || s.VectorCandidates > maxHybridCandidates {
		return s, hybridInvalid("VectorCandidates %d is outside 1..%d", s.VectorCandidates, maxHybridCandidates)
	}
	if s.RRFK < 1 {
		return s, hybridInvalid("RRFK %d must be at least 1", s.RRFK)
	}
	if s.Limit < 1 || s.Limit > maxHybridLimit {
		return s, hybridInvalid("Limit %d is outside 1..%d", s.Limit, maxHybridLimit)
	}
	dim := s.Vector.Dim()
	if dim == 0 {
		dim = len(s.QueryVector)
	}
	if err := s.QueryVector.Validate(dim); err != nil {
		return s, &Error{Kind: ErrValidation, Op: "hybrid search", Err: err}
	}
	return s, nil
}

// hybridSQL is the fused statement. The %s verbs take the quoted table, the
// primary key, the text column, the vector column and the soft-delete filter.
const hybridSQL = `WITH text_hits AS (
  SELECT pk, row_number() OVER (ORDER BY score DESC, pk ASC) AS rnk
  FROM (
    SELECT t.%[2]s AS pk, ts_rank_cd(to_tsvector($1::regconfig, t.%[3]s), websearch_to_tsquery($1::regconfig, $2)) AS score
    FROM %[1]s t
    WHERE to_tsvector($1::regconfig, t.%[3]s) @@ websearch_to_tsquery($1::regconfig, $2)%[5]s
    ORDER BY score DESC, t.%[2]s ASC
    LIMIT $3
  ) s
),
vec_hits AS (
  SELECT pk, row_number() OVER (ORDER BY dist ASC, pk ASC) AS rnk
  FROM (
    SELECT t.%[2]s AS pk, t.%[4]s <=> $4 AS dist
    FROM %[1]s t
    WHERE t.%[4]s IS NOT NULL%[6]s
    ORDER BY t.%[4]s <=> $4
    LIMIT $5
  ) s
  WHERE dist <> 'NaN'::float8
),
fused AS (
  SELECT COALESCE(a.pk, b.pk) AS pk,
         COALESCE(1.0 / ($6::int + a.rnk), 0) + COALESCE(1.0 / ($6::int + b.rnk), 0) AS score
  FROM text_hits a FULL OUTER JOIN vec_hits b ON a.pk = b.pk
)
SELECT %[7]s, f.score::float8
FROM fused f JOIN %[1]s t ON t.%[2]s = f.pk
ORDER BY f.score DESC, f.pk ASC
LIMIT $7`

// SQL compiles the statement without running it. The result has the seven
// bound arguments described at HybridSearch.
func (h HybridQuery[T]) SQL() (Compiled, error) {
	s, err := h.normalize()
	if err != nil {
		return Compiled{}, err
	}
	d := h.q.Dialect()
	ident := func(name string) string { return string(d.AppendIdent(nil, name)) }
	table := ident(h.meta.Table)
	if h.meta.Schema != "" {
		table = ident(h.meta.Schema) + "." + table
	}
	pk := ident(h.meta.Columns[h.meta.PK[0]].Name)

	var softText, softVec string
	if h.meta.SoftDelete >= 0 && h.meta.SoftDelete < len(h.meta.Columns) {
		var pb Builder
		pb.d = d
		softDeletePred(h.meta, "t", false).AppendSQL(&pb)
		softText = " AND " + string(pb.buf)
		softVec = " AND " + string(pb.buf)
	}
	cols := ""
	for i, c := range h.meta.Columns {
		if i > 0 {
			cols += ", "
		}
		cols += "t." + ident(c.Name)
	}
	sql := fmt.Sprintf(hybridSQL, table, pk, ident(s.Text.ColumnName()), ident(s.Vector.ColumnName()), softText, softVec, cols)
	args := []any{s.TextConfig, s.Query, s.TextCandidates, s.QueryVector, s.VectorCandidates, s.RRFK, s.Limit}
	sensitive := FlagsOf(s.Text)&FlagSensitive != 0 || FlagsOf(s.Vector)&FlagSensitive != 0
	return compile(d, func(b *Builder) {
		prev := b.argSens
		b.argSens = prev || sensitive
		b.appendRaw(sql, args)
		b.argSens = prev
	})
}

// All runs the query and returns the fused rows, best first.
func (h HybridQuery[T]) All(ctx context.Context) ([]HybridResult[T], error) {
	c, err := h.SQL()
	if err != nil {
		return nil, err
	}
	r, err := h.q.conn().query(ctx, "select", h.meta.Table, "", c)
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	var out []HybridResult[T]
	n := len(h.meta.Columns)
	dest := make([]any, n+1)
	for r.Next() {
		out = append(out, HybridResult[T]{})
		res := &out[len(out)-1]
		for i := 0; i < n; i++ {
			dest[i] = h.meta.Ptr(&res.Row, i)
		}
		dest[n] = &res.Score
		if err := r.Scan(dest...); err != nil {
			return nil, err
		}
		if err := afterLoad(ctx, h.meta, &res.Row); err != nil {
			return nil, err
		}
	}
	if err := r.Err(); err != nil {
		return nil, err
	}
	if err := r.Close(); err != nil {
		return nil, err
	}
	return out, nil
}
